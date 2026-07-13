package authprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

const discordAPIBase = "https://discord.com/api/v10"

// discordRequestsPerSecond はBotトークンでのAPI呼び出しの上限レートです。
// Discordのグローバル上限（およそ毎秒50）を十分下回る値にし、多人数の
// キャッシュ更新が偶然重なっても 429 を踏まないようにします。
const discordRequestsPerSecond = 25

// クールダウン（サーキットブレーカー）の各期間。
// Discordがエラーを返している間もリクエスト毎に呼び出し続けると、
// 無効トークンやレート制限に対して延々と叩き続けることになり、
// Discordの濫用検知（トークン強制リセット）を招く。失敗したら一定時間
// 呼び出し自体を止め、その間は期限切れキャッシュで凌ぐ。
const (
	// 認証エラー(401/403)はトークンが無効。再試行しても直らないため長く止める。
	discordAuthCooldown = 15 * time.Minute
	// その他の失敗（ネットワーク・5xx等）は指数的に延ばす。
	discordFailCooldownMin = 30 * time.Second
	discordFailCooldownMax = 5 * time.Minute
	// 429で Retry-After が読めなかった場合の既定待機。
	discordDefaultRetryAfter = 5 * time.Second
)

// ErrDiscordUnavailable はクールダウン中でDiscordを呼び出さなかったことを表します。
var ErrDiscordUnavailable = errors.New("discord APIの呼び出しを抑止中です")

// isPlaceholderToken は初回生成の config.yaml に残るひな型値かを判定します。
// ひな型のままDiscordへ接続すると、確実に失敗するIDENTIFY/APIコールを
// 撃ち続けることになるため、そもそも呼び出さない。
func isPlaceholderToken(token string) bool {
	return token == "" || strings.Contains(token, "YOUR_")
}

// discordUser はDiscordの `/users/@me` レスポンスを表します。
type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
}

// discordGuildMember はDiscordのギルドメンバーAPIレスポンスを表します。
type discordGuildMember struct {
	User  discordUser `json:"user"`
	Roles []string    `json:"roles"`
}

// discordErrorResponse はDiscord APIエラーレスポンスです。
type discordErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type roleCacheEntry struct {
	expiresAt time.Time
	roles     []string
	isMember  bool
}

// memberResult は singleflight で共有するメンバー取得結果です。
type memberResult struct {
	roles    []string
	isMember bool
}

// DiscordConfig はDiscordプロバイダーの設定を表します。
type DiscordConfig struct {
	Name         string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	GuildID      string
	BotToken     string
	// GatewayEnabled はゲートウェイ常時接続によるロールのリアルタイム同期を試みるかどうか。
	GatewayEnabled bool
}

// DiscordProvider はDiscordをこのアプリケーションで唯一のOAuth2実装として提供する
// Providerです。DiscordはOIDCを提供していないため、ログインはOAuth2、ロール/在籍の
// 参照はBot REST（キャッシュ付き）で Provider インターフェースを満たします。
// さらにゲートウェイ同期が有効な場合は、ロール/在籍参照をメモリ解決へ切り替えます。
type DiscordProvider struct {
	oauthConfig *oauth2.Config
	httpClient  *http.Client
	name        string
	guildID     string
	botToken    string
	// apiBase はDiscord REST APIのベースURL。テストでスタブへ差し替えるためにフィールド化する。
	apiBase string

	// gatewayEnabled はゲートウェイ同期を試みる設定。gateway は同期成功時に非同期で
	// セットされ、準備完了後は GetUserRoles/VerifyMembership がRESTではなくメモリ参照で
	// 解決される。バックグラウンド起動と参照が別ゴルーチンのため atomic で共有する。
	gatewayEnabled bool
	gateway        atomic.Pointer[gatewaySync]

	cacheMutex sync.RWMutex
	cache      map[string]*roleCacheEntry
	cacheTTL   time.Duration

	// limiter はDiscordへのライブ取得を毎秒レートで絞り、更新の殺到を平準化します。
	limiter *rate.Limiter
	// sf は同一ユーザーへの同時ライブ取得を1回のAPI呼び出しに集約します。
	sf singleflight.Group

	// cooldown はDiscordが失敗を返している間、呼び出し自体を止めるサーキットブレーカー。
	// レートリミッターは「毎秒25回まで許可」するだけなので、失敗が続くと毎秒25回
	// 叩き続けてしまう。抑止期間を設けて濫用にならないようにする。
	cooldownMu    sync.Mutex
	cooldownUntil time.Time
	cooldownStep  time.Duration
}

// inCooldown はクールダウン中かを返します。
func (p *DiscordProvider) inCooldown() (time.Time, bool) {
	p.cooldownMu.Lock()
	defer p.cooldownMu.Unlock()
	if time.Now().Before(p.cooldownUntil) {
		return p.cooldownUntil, true
	}
	return time.Time{}, false
}

// tripCooldown は指定期間だけDiscordへの呼び出しを抑止します。
func (p *DiscordProvider) tripCooldown(d time.Duration) {
	p.cooldownMu.Lock()
	defer p.cooldownMu.Unlock()
	until := time.Now().Add(d)
	if until.After(p.cooldownUntil) {
		p.cooldownUntil = until
	}
}

// tripFailCooldown は一時的失敗に対し、指数的に伸びる抑止期間を設定します。
func (p *DiscordProvider) tripFailCooldown() {
	p.cooldownMu.Lock()
	step := p.cooldownStep
	if step <= 0 {
		step = discordFailCooldownMin
	} else {
		step *= 2
		if step > discordFailCooldownMax {
			step = discordFailCooldownMax
		}
	}
	p.cooldownStep = step
	until := time.Now().Add(step)
	if until.After(p.cooldownUntil) {
		p.cooldownUntil = until
	}
	p.cooldownMu.Unlock()
}

// clearCooldown は成功時に抑止状態を解除します。
func (p *DiscordProvider) clearCooldown() {
	p.cooldownMu.Lock()
	p.cooldownUntil = time.Time{}
	p.cooldownStep = 0
	p.cooldownMu.Unlock()
}

// NewDiscordProvider はDiscordProviderを作成します。
func NewDiscordProvider(cfg DiscordConfig) *DiscordProvider {
	name := cfg.Name
	if name == "" {
		name = "discord"
	}

	return &DiscordProvider{
		name: name,
		oauthConfig: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"identify", "email", "guilds.members.read"},
			Endpoint: oauth2.Endpoint{
				AuthURL: "https://discord.com/api/oauth2/authorize",
				// #nosec G101 - OAuth2のトークンエンドポイントURLであり資格情報ではない
				TokenURL: "https://discord.com/api/oauth2/token",
			},
		},
		guildID:        cfg.GuildID,
		botToken:       cfg.BotToken,
		apiBase:        discordAPIBase,
		gatewayEnabled: cfg.GatewayEnabled,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		cache:          make(map[string]*roleCacheEntry),
		cacheTTL:       5 * time.Minute,
		limiter:        rate.NewLimiter(rate.Limit(discordRequestsPerSecond), discordRequestsPerSecond),
	}
}

// StartMembershipSync はゲートウェイ同期を開始します。
// started=true はリアルタイム同期が有効化されたこと、false は（無効設定・資格情報不足・
// インテント未許可などで）RESTフォールバックのままであることを表します。
// onChange はメンバーのロール変化時に該当userIDで呼ばれます（SSEの権限再解決などに使用）。
func (p *DiscordProvider) StartMembershipSync(ctx context.Context, onChange func(string)) (bool, error) {
	// ひな型トークンのままでは必ず認証失敗(4004)する。無駄なIDENTIFYを撃たない。
	if !p.gatewayEnabled || isPlaceholderToken(p.botToken) || p.guildID == "" {
		return false, nil
	}

	gs, err := newGatewaySync(p.botToken, p.guildID, onChange)
	if err != nil {
		return false, err
	}

	// Ready（全メンバー同期完了）まで最大30秒待つ。到達しなければ自動でRESTへ戻す。
	if err := gs.Start(ctx, 30*time.Second); err != nil {
		return false, err
	}

	p.gateway.Store(gs)
	return true, nil
}

// StopMembershipSync はゲートウェイ接続を閉じます（未起動なら何もしません）。
func (p *DiscordProvider) StopMembershipSync() error {
	if gs := p.gateway.Load(); gs != nil {
		return gs.Close()
	}
	return nil
}

// ttlWithJitter はキャッシュ有効期限に 0〜TTL/5 のランダムな上乗せをして返します。
// 同時刻にログイン/接続した多数のエントリの期限が揃って一斉更新（サンダリングハード）に
// なるのを防ぎ、更新タイミングを時間方向へ散らします。
func (p *DiscordProvider) ttlWithJitter() time.Duration {
	spread := int64(p.cacheTTL / 5)
	if spread <= 0 {
		return p.cacheTTL
	}
	return p.cacheTTL + time.Duration(rand.Int64N(spread))
}

// Name はプロバイダー名を返します。
func (p *DiscordProvider) Name() string {
	return p.name
}

// AuthCodeURL は認可URLを返します。
func (p *DiscordProvider) AuthCodeURL(state string) string {
	return p.oauthConfig.AuthCodeURL(state)
}

// Exchange は認可コードをトークンと交換します。
func (p *DiscordProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.oauthConfig.Exchange(ctx, code)
}

// FetchUserInfo はDiscordの `/users/@me` からユーザー情報を取得します。
func (p *DiscordProvider) FetchUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("レスポンスボディのクローズに失敗", "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord APIエラー: %d", resp.StatusCode)
	}

	var du discordUser
	if err := json.NewDecoder(resp.Body).Decode(&du); err != nil {
		return nil, err
	}

	return &UserInfo{
		Provider: p.name,
		Subject:  du.ID,
		Username: du.Username,
		Avatar:   du.Avatar,
		Email:    du.Email,
	}, nil
}

// IsMember は指定されたギルドのメンバーかどうかを確認します。
func (p *DiscordProvider) IsMember(ctx context.Context, token *oauth2.Token, _ *UserInfo) (bool, error) {
	url := fmt.Sprintf("https://discord.com/api/users/@me/guilds/%s/member", p.guildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("レスポンスボディのクローズに失敗", "error", closeErr)
		}
	}()

	return resp.StatusCode == http.StatusOK, nil
}

// PrecreateUserDirectory はDiscordではtrueを返します。
// ギルドメンバーに限定されているため、ログイン時にユーザーディレクトリを事前作成します。
func (p *DiscordProvider) PrecreateUserDirectory() bool {
	return true
}

// GetUserRoles はギルドメンバーのロールID一覧を返します。
// ゲートウェイ同期が準備完了ならメモリから即時に、そうでなければBotトークンで
// REST取得します(5分キャッシュ)。
func (p *DiscordProvider) GetUserRoles(ctx context.Context, subject string) ([]string, error) {
	if gs := p.gateway.Load(); gs != nil {
		if roles, present, ok := gs.lookup(subject); ok {
			if !present {
				return nil, fmt.Errorf("ユーザーがギルドに存在しません (user_id: %s)", subject)
			}
			return roles, nil
		}
	}

	roles, isMember, err := p.fetchMember(ctx, subject)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, fmt.Errorf("ユーザーがギルドに存在しません (user_id: %s)", subject)
	}
	return roles, nil
}

// VerifyMembership はギルド在籍を継続確認します。
// ゲートウェイ同期が準備完了ならメモリから即時に、そうでなければBotトークンで
// REST確認します(5分キャッシュ)。
func (p *DiscordProvider) VerifyMembership(ctx context.Context, subject string) (bool, error) {
	if gs := p.gateway.Load(); gs != nil {
		if _, present, ok := gs.lookup(subject); ok {
			return present, nil
		}
	}

	_, isMember, err := p.fetchMember(ctx, subject)
	if err != nil {
		return false, err
	}
	return isMember, nil
}

// fetchMember はギルドメンバー情報（ロールと在籍有無）を取得します(5分キャッシュ)。
// ギルドに存在しない場合は isMember=false を返し、エラーとはしません。
// ライブ取得が一時的に失敗した場合は、期限切れのキャッシュがあればそれを返します
// （stale-while-error。Discord障害やレート制限で全面停止しないため）。
func (p *DiscordProvider) fetchMember(ctx context.Context, subject string) (roles []string, isMember bool, err error) {
	// キャッシュを読む（期限切れでも stale-while-error 用に保持する）
	p.cacheMutex.RLock()
	entry, exists := p.cache[subject]
	var (
		staleRoles  []string
		staleMember bool
		fresh       bool
	)
	if exists {
		staleRoles = make([]string, len(entry.roles))
		copy(staleRoles, entry.roles)
		staleMember = entry.isMember
		fresh = time.Now().Before(entry.expiresAt)
	}
	p.cacheMutex.RUnlock()

	if exists && fresh {
		return staleRoles, staleMember, nil
	}

	// 同一ユーザーへの同時ミスは1回のライブ取得に集約する（重複APIコールの排除）。
	v, err, _ := p.sf.Do(subject, func() (interface{}, error) {
		r, m, liveErr := p.fetchMemberLive(ctx, subject)
		if liveErr != nil {
			return nil, liveErr
		}
		return memberResult{roles: r, isMember: m}, nil
	})
	if err != nil {
		if exists {
			slog.Warn("Discord APIの取得に失敗したため期限切れキャッシュを使用します（stale-while-error）",
				"user_id", subject, "error", err)
			return staleRoles, staleMember, nil
		}
		return nil, false, err
	}
	res, ok := v.(memberResult)
	if !ok {
		return nil, false, fmt.Errorf("メンバー取得結果の型が不正です")
	}
	return res.roles, res.isMember, nil
}

// fetchMemberLive はキャッシュを介さずDiscord APIからメンバー情報を取得し、結果をキャッシュします。
// ギルドに存在しない場合は isMember=false を返し、エラーとはしません。
func (p *DiscordProvider) fetchMemberLive(ctx context.Context, subject string) (roles []string, isMember bool, err error) {
	// ひな型トークンのままなら確実に失敗する。ネットワークへ出さない。
	if isPlaceholderToken(p.botToken) {
		return nil, false, fmt.Errorf("%w: bot_token が未設定です", ErrDiscordUnavailable)
	}

	// 失敗が続いている間は呼び出さない（濫用防止のサーキットブレーカー）。
	if until, ok := p.inCooldown(); ok {
		return nil, false, fmt.Errorf("%w (%s まで)", ErrDiscordUnavailable, until.Format(time.RFC3339))
	}

	// グローバルレート上限を超えないよう、送信前にトークンの発行を待つ。
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, false, fmt.Errorf("レート制限待機の中断: %w", err)
	}

	url := fmt.Sprintf("%s/guilds/%s/members/%s", p.apiBase, p.guildID, subject)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("リクエスト作成エラー: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+p.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.tripFailCooldown()
		return nil, false, fmt.Errorf("discord API呼び出しエラー: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("レスポンスボディのクローズに失敗", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.tripFailCooldown()
		return nil, false, fmt.Errorf("レスポンス読み取りエラー: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// この後でメンバー情報をパースする（下の共通処理へ抜ける）。
	case http.StatusNotFound:
		// ギルドに存在しない（退出済み等）。在籍なしとしてキャッシュする。
		p.clearCooldown()
		p.cacheMutex.Lock()
		p.cache[subject] = &roleCacheEntry{expiresAt: time.Now().Add(p.ttlWithJitter()), isMember: false}
		p.cacheMutex.Unlock()
		return nil, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		// トークンが無効。再試行しても直らないため長時間停止する。
		// これを止めないと全リクエストがDiscordを叩き、濫用検知でトークンを失う。
		p.tripCooldown(discordAuthCooldown)
		slog.Error("Discord Botトークンが拒否されました。config.yaml の bot_token を確認してください。以後しばらくDiscordへの呼び出しを停止します",
			"status", resp.StatusCode, "cooldown", discordAuthCooldown.String())
		return nil, false, fmt.Errorf("discord Bot認証エラー (status: %d)", resp.StatusCode)
	case http.StatusTooManyRequests:
		// Discordの指示（Retry-After）に従って待つ。従わないと429を誘発し続ける。
		wait := retryAfter(resp, body)
		p.tripCooldown(wait)
		slog.Warn("Discord APIにレート制限されました。指定時間だけ呼び出しを停止します", "retry_after", wait.String())
		return nil, false, fmt.Errorf("discord APIレート制限 (status: 429, retry_after: %s)", wait)
	default:
		p.tripFailCooldown()
		var errResp discordErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, false, fmt.Errorf("discord APIエラー (code: %d, message: %s)", errResp.Code, errResp.Message)
		}
		return nil, false, fmt.Errorf("discord APIエラー (status: %d)", resp.StatusCode)
	}

	// 成功したので抑止状態を解除する。
	p.clearCooldown()

	var member discordGuildMember
	if err := json.Unmarshal(body, &member); err != nil {
		return nil, false, fmt.Errorf("JSONパースエラー: %w", err)
	}

	slog.Info("Discord APIからロール情報を取得",
		"user_id", subject,
		"username", member.User.Username,
		"roles", member.Roles,
		"role_count", len(member.Roles))

	p.cacheMutex.Lock()
	p.cache[subject] = &roleCacheEntry{
		roles:     member.Roles,
		expiresAt: time.Now().Add(p.ttlWithJitter()),
		isMember:  true,
	}
	p.cacheMutex.Unlock()

	return member.Roles, true, nil
}

// retryAfter は429レスポンスから待機時間を求めます。
// ヘッダ `Retry-After`（秒）を優先し、無ければボディの `retry_after` を見ます。
// どちらも読めない場合は既定値を返します（0秒待ちで即再試行しないため）。
func retryAfter(resp *http.Response, body []byte) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if sec, err := strconv.ParseFloat(v, 64); err == nil && sec > 0 {
			return time.Duration(sec * float64(time.Second))
		}
	}
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.RetryAfter > 0 {
		return time.Duration(payload.RetryAfter * float64(time.Second))
	}
	return discordDefaultRetryAfter
}

// ClearCache はロールキャッシュをクリアします（テスト用）。
func (p *DiscordProvider) ClearCache() {
	p.cacheMutex.Lock()
	defer p.cacheMutex.Unlock()
	p.cache = make(map[string]*roleCacheEntry)
}

// SetCacheTTL はキャッシュTTLを設定します（テスト用）。
func (p *DiscordProvider) SetCacheTTL(ttl time.Duration) {
	p.cacheTTL = ttl
}
