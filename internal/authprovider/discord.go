package authprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const discordAPIBase = "https://discord.com/api/v10"

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

// DiscordConfig はDiscordプロバイダーの設定を表します。
type DiscordConfig struct {
	Name         string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	GuildID      string
	BotToken     string
}

// DiscordProvider はDiscordをこのアプリケーションで唯一のOAuth2実装として提供する
// Providerです。DiscordはOIDCを提供していないため、OAuth2 + REST APIで
// Provider インターフェースを満たします。
type DiscordProvider struct {
	oauthConfig *oauth2.Config
	httpClient  *http.Client
	name        string
	guildID     string
	botToken    string

	cacheMutex sync.RWMutex
	cache      map[string]*roleCacheEntry
	cacheTTL   time.Duration
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
		guildID:    cfg.GuildID,
		botToken:   cfg.BotToken,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]*roleCacheEntry),
		cacheTTL:   5 * time.Minute,
	}
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

// GetUserRoles はBotトークンを使いギルドメンバーのロールID一覧を取得します(5分キャッシュ)。
func (p *DiscordProvider) GetUserRoles(ctx context.Context, subject string) ([]string, error) {
	roles, isMember, err := p.fetchMember(ctx, subject)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, fmt.Errorf("ユーザーがギルドに存在しません (user_id: %s)", subject)
	}
	return roles, nil
}

// VerifyMembership はBotトークンでギルド在籍を確認します(5分キャッシュ)。
// ログイン後の各リクエストで継続的に在籍を検証するために使用します。
func (p *DiscordProvider) VerifyMembership(ctx context.Context, subject string) (bool, error) {
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

	roles, isMember, err = p.fetchMemberLive(ctx, subject)
	if err != nil {
		if exists {
			slog.Warn("Discord APIの取得に失敗したため期限切れキャッシュを使用します（stale-while-error）",
				"user_id", subject, "error", err)
			return staleRoles, staleMember, nil
		}
		return nil, false, err
	}
	return roles, isMember, nil
}

// fetchMemberLive はキャッシュを介さずDiscord APIからメンバー情報を取得し、結果をキャッシュします。
// ギルドに存在しない場合は isMember=false を返し、エラーとはしません。
func (p *DiscordProvider) fetchMemberLive(ctx context.Context, subject string) (roles []string, isMember bool, err error) {
	url := fmt.Sprintf("%s/guilds/%s/members/%s", discordAPIBase, p.guildID, subject)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("リクエスト作成エラー: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+p.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("discord API呼び出しエラー: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("レスポンスボディのクローズに失敗", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("レスポンス読み取りエラー: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// この後でメンバー情報をパースする（下の共通処理へ抜ける）。
	case http.StatusNotFound:
		// ギルドに存在しない（退出済み等）。在籍なしとしてキャッシュする。
		p.cacheMutex.Lock()
		p.cache[subject] = &roleCacheEntry{expiresAt: time.Now().Add(p.cacheTTL), isMember: false}
		p.cacheMutex.Unlock()
		return nil, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, false, fmt.Errorf("discord Bot認証エラー (status: %d)", resp.StatusCode)
	case http.StatusTooManyRequests:
		var errResp discordErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, false, fmt.Errorf("discord APIレート制限: %s", errResp.Message)
		}
		return nil, false, fmt.Errorf("discord APIレート制限 (status: 429)")
	default:
		var errResp discordErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, false, fmt.Errorf("discord APIエラー (code: %d, message: %s)", errResp.Code, errResp.Message)
		}
		return nil, false, fmt.Errorf("discord APIエラー (status: %d)", resp.StatusCode)
	}

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
		expiresAt: time.Now().Add(p.cacheTTL),
		isMember:  true,
	}
	p.cacheMutex.Unlock()

	return member.Roles, true, nil
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
