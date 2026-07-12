package authprovider

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func newTestGateway() (*gatewaySync, *[]string) {
	changed := &[]string{}
	gs := &gatewaySync{
		guildID: "g",
		members: make(map[string][]string),
		onChange: func(id string) {
			*changed = append(*changed, id)
		},
	}
	return gs, changed
}

func member(id string, roles ...string) *discordgo.Member {
	return &discordgo.Member{GuildID: "g", User: &discordgo.User{ID: id}, Roles: roles}
}

func TestGatewayLookupBeforeReady(t *testing.T) {
	gs, _ := newTestGateway()
	if _, _, ok := gs.lookup("u1"); ok {
		t.Error("同期未完了では ok=false であるべき")
	}
}

func TestGatewayChunkPopulatesAndReadies(t *testing.T) {
	gs, _ := newTestGateway()
	gs.handleChunk(nil, &discordgo.GuildMembersChunk{
		GuildID:    "g",
		Members:    []*discordgo.Member{member("u1", "r1", "r2")},
		ChunkIndex: 0,
		ChunkCount: 1,
	})

	roles, present, ok := gs.lookup("u1")
	if !ok || !present {
		t.Fatalf("同期完了後は ok=present=true を期待 (ok=%v present=%v)", ok, present)
	}
	if len(roles) != 2 || roles[0] != "r1" || roles[1] != "r2" {
		t.Errorf("ロールが一致しない: %v", roles)
	}

	// 同期済みだが未在籍のユーザーは present=false
	if _, present, ok := gs.lookup("nobody"); !ok || present {
		t.Errorf("未在籍ユーザーは ok=true present=false を期待 (ok=%v present=%v)", ok, present)
	}
}

func TestGatewayMultiChunkReadyOnLast(t *testing.T) {
	gs, _ := newTestGateway()
	gs.handleChunk(nil, &discordgo.GuildMembersChunk{GuildID: "g", Members: []*discordgo.Member{member("u1")}, ChunkIndex: 0, ChunkCount: 2})
	if gs.ready.Load() {
		t.Error("最終チャンク前に ready になってはいけない")
	}
	gs.handleChunk(nil, &discordgo.GuildMembersChunk{GuildID: "g", Members: []*discordgo.Member{member("u2")}, ChunkIndex: 1, ChunkCount: 2})
	if !gs.ready.Load() {
		t.Error("最終チャンク後に ready になるべき")
	}
}

func TestGatewayUpdateAndRemoveNotifyAfterReady(t *testing.T) {
	gs, changed := newTestGateway()
	gs.handleChunk(nil, &discordgo.GuildMembersChunk{GuildID: "g", Members: []*discordgo.Member{member("u1", "r1")}, ChunkIndex: 0, ChunkCount: 1})

	gs.handleMemberUpdate(nil, &discordgo.GuildMemberUpdate{Member: member("u1", "r1", "admin")})
	if roles, _, _ := gs.lookup("u1"); len(roles) != 2 {
		t.Errorf("更新後のロール数が違う: %v", roles)
	}

	gs.handleMemberRemove(nil, &discordgo.GuildMemberRemove{Member: member("u1")})
	if _, present, _ := gs.lookup("u1"); present {
		t.Error("削除後は present=false であるべき")
	}

	// 準備完了後の更新・削除で onChange が呼ばれている（初回チャンクでは呼ばれない）
	if len(*changed) != 2 {
		t.Errorf("onChange 呼び出し回数は2を期待、実際=%d (%v)", len(*changed), *changed)
	}
}

func TestGatewayNoNotifyBeforeReady(t *testing.T) {
	gs, changed := newTestGateway()
	// ready 前の add は通知しない
	gs.upsert(member("u1", "r1"))
	if len(*changed) != 0 {
		t.Errorf("ready 前は onChange を呼ばないべき、実際=%d", len(*changed))
	}
}

func TestGatewayIgnoresOtherGuild(t *testing.T) {
	gs, _ := newTestGateway()
	other := &discordgo.Member{GuildID: "other", User: &discordgo.User{ID: "x"}, Roles: []string{"r"}}
	gs.handleChunk(nil, &discordgo.GuildMembersChunk{GuildID: "other", Members: []*discordgo.Member{other}, ChunkIndex: 0, ChunkCount: 1})
	if gs.ready.Load() {
		t.Error("別ギルドのチャンクで ready にしてはいけない")
	}
}
