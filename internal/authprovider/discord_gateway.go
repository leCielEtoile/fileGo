package authprovider

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

// errGatewayNotReady はゲートウェイが所定時間内に全メンバー同期を完了できなかったことを表します。
// Server Members Intent 未許可（close code 4014）や接続不良でReadyに至らない場合に発生し、
// 呼び出し側はRESTフォールバックへ切り替えます。
var errGatewayNotReady = errors.New("ゲートウェイが同期完了しませんでした")

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

	gs := &gatewaySync{
		session:  s,
		guildID:  guildID,
		onChange: onChange,
		members:  make(map[string][]string),
	}

	s.AddHandler(gs.handleGuildCreate)
	s.AddHandler(gs.handleChunk)
	s.AddHandler(gs.handleMemberAdd)
	s.AddHandler(gs.handleMemberUpdate)
	s.AddHandler(gs.handleMemberRemove)

	return gs, nil
}

// Start はゲートウェイへ接続し、全メンバー同期の完了（Ready）を待ちます。
// readyTimeout 内に完了しなければ接続を閉じ errGatewayNotReady を返します。
// これによりインテント未許可（Readyに到達しない）を検出し、呼び出し側がフォールバックできます。
func (gs *gatewaySync) Start(ctx context.Context, readyTimeout time.Duration) error {
	if err := gs.session.Open(); err != nil {
		return err
	}

	deadline := time.NewTimer(readyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = gs.session.Close()
			return ctx.Err()
		case <-deadline.C:
			_ = gs.session.Close()
			return errGatewayNotReady
		case <-tick.C:
			if gs.ready.Load() {
				return nil
			}
		}
	}
}

// Close はゲートウェイ接続を閉じます。
func (gs *gatewaySync) Close() error {
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
	if e.Member == nil || e.Member.GuildID != gs.guildID || e.Member.User == nil {
		return
	}
	gs.mu.Lock()
	delete(gs.members, e.Member.User.ID)
	gs.mu.Unlock()
	gs.notify(e.Member.User.ID)
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
