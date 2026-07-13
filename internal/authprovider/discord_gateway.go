package authprovider

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

// errGatewayNotReady はゲートウェイが所定時間内に全メンバー同期を完了できなかったことを表します。
// Server Members Intent 未許可（close code 4014）や接続不良でReadyに至らない場合に発生し、
// 呼び出し側はRESTフォールバックへ切り替えます。
var errGatewayNotReady = errors.New("ゲートウェイが同期完了しませんでした")

// 再接続のバックオフ範囲。
const (
	gatewayReconnectMin = 5 * time.Second
	gatewayReconnectMax = 5 * time.Minute
	// 上限に達したら諦めてRESTへ委ねる（撃ち続けない）。
	gatewayReconnectMaxAttempts = 6
)

// fatalGatewayCloseCodes は再接続しても回復し得ないcloseコードです。
// discordgo は既定（ShouldReconnectOnError=true）でこれらのコードでも無限に
// 再接続を試み、成功し得ないIDENTIFYを撃ち続けてDiscordの濫用検知
// （Botトークンの強制リセット）を招く。そのため自前で判定して打ち切る。
var fatalGatewayCloseCodes = map[int]string{
	4004: "認証失敗（Botトークンが無効）",
	4010: "無効なシャード",
	4011: "シャーディングが必要",
	4012: "無効なAPIバージョン",
	4013: "無効なインテント",
	4014: "許可されていないインテント（Server Members Intent が未有効）",
}

// fatalGatewayClose は再接続を打ち切るべきエラーかを判定します。
func fatalGatewayClose(err error) (reason string, fatal bool) {
	if err == nil {
		return "", false
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		if r, ok := fatalGatewayCloseCodes[ce.Code]; ok {
			return r, true
		}
		return "", false
	}
	// CloseError としてラップされずに返る場合に備えた保険。
	msg := err.Error()
	for code, r := range fatalGatewayCloseCodes {
		if strings.Contains(msg, "close "+strconv.Itoa(code)) {
			return r, true
		}
	}
	return "", false
}

// gatewaySync はDiscordゲートウェイに常時接続し、ギルドメンバーのロールを
// メモリ上に保持してリアルタイムに追随させます。準備完了後は、ロール参照が
// REST APIを介さずメモリ参照だけで済み、レート制限とも無縁になります。
type gatewaySync struct {
	session  *discordgo.Session
	guildID  string
	onChange func(userID string)

	mu      sync.RWMutex
	members map[string][]string // userID -> ロールID
	ready   atomic.Bool

	// 再接続は discordgo 任せにせず自前で管理する（無限再接続を避けるため）。
	reconnectCh chan struct{}
	stopCh      chan struct{}
	stopped     atomic.Bool
}

// newGatewaySync はゲートウェイ同期を初期化します（まだ接続はしません）。
// onChange はメンバーのロール変化時に該当userIDで呼ばれます（同期完了後のみ）。
func newGatewaySync(botToken, guildID string, onChange func(string)) (*gatewaySync, error) {
	s, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return nil, err
	}
	// ギルドとメンバー（特権インテント）を購読する。メンバーイベントの受信に必須。
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers

	// discordgo の自動再接続は致命的closeコードでも無限に再試行するため無効化し、
	// 再接続は supervise() で「致命的なら即中止・上限付きバックオフ」で行う。
	s.ShouldReconnectOnError = false

	gs := &gatewaySync{
		session:     s,
		guildID:     guildID,
		onChange:    onChange,
		members:     make(map[string][]string),
		reconnectCh: make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
	}

	s.AddHandler(gs.handleGuildCreate)
	s.AddHandler(gs.handleChunk)
	s.AddHandler(gs.handleMemberAdd)
	s.AddHandler(gs.handleMemberUpdate)
	s.AddHandler(gs.handleMemberRemove)
	s.AddHandler(gs.handleDisconnect)

	return gs, nil
}

// handleDisconnect は切断を検知し、再接続を supervise() へ依頼します。
// 切断中はメモリのロール表が古くなるため ready を下ろし、RESTへ委ねます。
func (gs *gatewaySync) handleDisconnect(_ *discordgo.Session, _ *discordgo.Disconnect) {
	if gs.stopped.Load() {
		return
	}
	gs.ready.Store(false)
	select {
	case gs.reconnectCh <- struct{}{}:
	default: // 既に再接続要求が積まれている
	}
}

// supervise は切断を検知して再接続します（Ready到達後に起動）。
// 再接続できないまま上限に達した、または致命的closeだった場合は諦めて
// 接続を閉じ、以降は REST 方式にフォールバックします（撃ち続けない）。
func (gs *gatewaySync) supervise() {
	for {
		select {
		case <-gs.stopCh:
			return
		case <-gs.reconnectCh:
			if gs.reconnectWithBackoff() {
				continue
			}
			gs.closeQuietly()
			slog.Error("ゲートウェイへ再接続できないため、以後はREST方式で動作します")
			return
		}
	}
}

// reconnectWithBackoff は指数バックオフで再接続を試みます。
// 致命的closeコードの場合は無駄なIDENTIFYを撃たないよう即座に諦めます。
func (gs *gatewaySync) reconnectWithBackoff() bool {
	backoff := gatewayReconnectMin
	for attempt := 1; attempt <= gatewayReconnectMaxAttempts; attempt++ {
		select {
		case <-gs.stopCh:
			return false
		case <-time.After(backoff):
		}

		err := gs.session.Open()
		if err == nil {
			slog.Info("ゲートウェイへ再接続しました", "attempt", attempt)
			return true
		}
		if reason, fatal := fatalGatewayClose(err); fatal {
			slog.Error("ゲートウェイの致命的エラーのため再接続を中止します", "reason", reason, "error", err)
			return false
		}

		slog.Warn("ゲートウェイの再接続に失敗しました", "attempt", attempt, "error", err, "next_wait", backoff.String())
		backoff *= 2
		if backoff > gatewayReconnectMax {
			backoff = gatewayReconnectMax
		}
	}
	return false
}

// Start はゲートウェイへ接続し、全メンバー同期の完了（Ready）を待ちます。
// readyTimeout 内に完了しなければ接続を閉じ errGatewayNotReady を返します。
// これによりインテント未許可（Readyに到達しない）を検出し、呼び出し側がフォールバックできます。
func (gs *gatewaySync) Start(ctx context.Context, readyTimeout time.Duration) error {
	if err := gs.session.Open(); err != nil {
		// 失敗しても内部ゴルーチンが残り得るため必ず後始末する。
		// 放置するとDiscordへ接続を試み続け、濫用検知の原因になる。
		gs.closeQuietly()
		return err
	}

	deadline := time.NewTimer(readyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			gs.closeQuietly()
			return ctx.Err()
		case <-deadline.C:
			gs.closeQuietly()
			return errGatewayNotReady
		case <-tick.C:
			if gs.ready.Load() {
				// 以後の切断は自前で監視・再接続する。
				go gs.supervise()
				return nil
			}
		}
	}
}

// closeQuietly はセッションを閉じ、失敗はログのみに留めます（クリーンアップ経路のため）。
func (gs *gatewaySync) closeQuietly() {
	if err := gs.session.Close(); err != nil {
		slog.Debug("ゲートウェイのクローズに失敗しました", "error", err)
	}
}

// Close はゲートウェイ接続を閉じ、再接続の監視も停止します。
// 二重呼び出しでもチャネルを二重クローズしないよう保護します。
func (gs *gatewaySync) Close() error {
	if gs.stopped.Swap(true) {
		return nil
	}
	close(gs.stopCh)
	gs.ready.Store(false)
	if gs.session == nil {
		return nil
	}
	return gs.session.Close()
}

// lookup はメンバーのロールを返します。
// ok=false は「まだ同期未完了」を意味し、呼び出し側はRESTへフォールバックします。
// ok=true かつ present=false は「同期済みだがギルド未在籍」を意味します。
func (gs *gatewaySync) lookup(userID string) (roles []string, present, ok bool) {
	if !gs.ready.Load() {
		return nil, false, false
	}
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	r, exists := gs.members[userID]
	if !exists {
		return nil, false, true
	}
	// 呼び出し側での変更から守るため複製を返す。
	out := make([]string, len(r))
	copy(out, r)
	return out, true, true
}

// handleGuildCreate は対象ギルドが利用可能になった時点で全メンバーの取得を要求します。
// Ready直後よりもギルドの状態が整っており確実です。応答は GuildMembersChunk として
// 届きます（handleChunk が受信）。
func (gs *gatewaySync) handleGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	if g.ID != gs.guildID {
		return
	}
	if err := s.RequestGuildMembers(gs.guildID, "", 0, "", false); err != nil {
		slog.Error("ギルドメンバー一括要求に失敗しました", "error", err, "guild_id", gs.guildID)
	}
}

// handleChunk は一括取得の各チャンクを取り込み、最終チャンクで準備完了とします。
func (gs *gatewaySync) handleChunk(_ *discordgo.Session, c *discordgo.GuildMembersChunk) {
	if c.GuildID != gs.guildID {
		return
	}
	gs.mu.Lock()
	for _, m := range c.Members {
		if m.User != nil {
			gs.members[m.User.ID] = m.Roles
		}
	}
	gs.mu.Unlock()

	// chunk_index は 0 始まり。最終チャンク受信で全件そろう。
	if c.ChunkIndex >= c.ChunkCount-1 {
		gs.ready.Store(true)
		slog.Info("ゲートウェイのメンバー同期が完了しました", "guild_id", gs.guildID, "members", gs.count())
	}
}

func (gs *gatewaySync) handleMemberAdd(_ *discordgo.Session, e *discordgo.GuildMemberAdd) {
	gs.upsert(e.Member)
}

func (gs *gatewaySync) handleMemberUpdate(_ *discordgo.Session, e *discordgo.GuildMemberUpdate) {
	gs.upsert(e.Member)
}

func (gs *gatewaySync) handleMemberRemove(_ *discordgo.Session, e *discordgo.GuildMemberRemove) {
	if e.Member == nil || e.User == nil || e.GuildID != gs.guildID {
		return
	}
	gs.mu.Lock()
	delete(gs.members, e.User.ID)
	gs.mu.Unlock()
	gs.notify(e.User.ID)
}

// upsert はメンバーのロールを更新し、同期完了後は変更を通知します。
func (gs *gatewaySync) upsert(m *discordgo.Member) {
	if m == nil || m.GuildID != gs.guildID || m.User == nil {
		return
	}
	gs.mu.Lock()
	gs.members[m.User.ID] = m.Roles
	gs.mu.Unlock()
	gs.notify(m.User.ID)
}

// notify は同期完了後にのみ、ロール変更を購読者へ伝えます。
// （初回一括取り込み中は大量イベントで無用な再計算を誘発しないよう抑止する）
func (gs *gatewaySync) notify(userID string) {
	if !gs.ready.Load() || gs.onChange == nil {
		return
	}
	gs.onChange(userID)
}

func (gs *gatewaySync) count() int {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return len(gs.members)
}
