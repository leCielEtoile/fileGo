package authprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestHasRequiredRole(t *testing.T) {
	cases := []struct {
		name     string
		required []string
		roles    []string
		want     bool
	}{
		{"要件なしなら常に許可", nil, nil, true},
		{"要件なしならロール無しでも許可", []string{}, []string{}, true},
		{"いずれか1つ保有で許可(OR)", []string{"r1", "r2"}, []string{"r2"}, true},
		{"複数保有でも許可", []string{"r1"}, []string{"r9", "r1"}, true},
		{"未保有は拒否", []string{"r1", "r2"}, []string{"r9"}, false},
		{"ロールが空なら拒否", []string{"r1"}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasRequiredRole(c.required, c.roles); got != c.want {
				t.Errorf("hasRequiredRole(%v, %v) = %v, want %v", c.required, c.roles, got, c.want)
			}
		})
	}
}

// ログイン時: 在籍していても必要ロールが無ければ拒否すること。
func TestDiscordIsMemberRequiresRole(t *testing.T) {
	// /users/@me/guilds/{g}/member は在籍していればロール付きの200を返す
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"u1"},"roles":["member"]}`))
	}))
	defer srv.Close()

	tok := &oauth2.Token{AccessToken: "t"}
	info := &UserInfo{Subject: "u1"}

	// 要件なし → 在籍のみでログイン可（従来動作）
	p := NewDiscordProvider(DiscordConfig{BotToken: "tok", GuildID: "g"})
	p.apiBase = srv.URL
	if ok, err := p.IsMember(context.Background(), tok, info); err != nil || !ok {
		t.Fatalf("要件なしでは在籍のみで許可されるべき: ok=%v err=%v", ok, err)
	}

	// 必要ロールを保有 → 許可
	p = NewDiscordProvider(DiscordConfig{BotToken: "tok", GuildID: "g", RequiredRoles: []string{"member"}})
	p.apiBase = srv.URL
	if ok, err := p.IsMember(context.Background(), tok, info); err != nil || !ok {
		t.Fatalf("必要ロール保有なら許可されるべき: ok=%v err=%v", ok, err)
	}

	// 必要ロールを未保有 → 在籍していても拒否
	p = NewDiscordProvider(DiscordConfig{BotToken: "tok", GuildID: "g", RequiredRoles: []string{"approved"}})
	p.apiBase = srv.URL
	ok, err := p.IsMember(context.Background(), tok, info)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if ok {
		t.Error("必要ロール未保有なのにログインが許可された（公開サーバーで誰でも入れてしまう）")
	}
}

// 未在籍は（ロール要件の有無に関わらず）拒否されること。
func TestDiscordIsMemberRejectsNonMember(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewDiscordProvider(DiscordConfig{BotToken: "tok", GuildID: "g", RequiredRoles: []string{"approved"}})
	p.apiBase = srv.URL
	if ok, _ := p.IsMember(context.Background(), &oauth2.Token{AccessToken: "t"}, &UserInfo{Subject: "u1"}); ok {
		t.Error("未在籍なのに許可された")
	}
}

// OIDC: required_roles は groups_claim 由来のロールと照合され、
// allowlist と併用時は両方満たす必要がある（AND）。
func TestOIDCIsMemberRequiresRole(t *testing.T) {
	newOIDC := func(required, domains []string) *OIDCProvider {
		return &OIDCProvider{
			requiredRoles:       required,
			allowedEmailDomains: normalizeStrings(domains),
			roleCache: map[string]roleCacheRecord{
				"u1": {roles: []string{"approved"}, updatedAt: time.Now()},
				"u2": {roles: []string{"guest"}, updatedAt: time.Now()},
			},
		}
	}
	ctx := context.Background()
	approved := &UserInfo{Subject: "u1", Email: "a@example.com"}
	guest := &UserInfo{Subject: "u2", Email: "b@example.com"}

	// 要件なし → 全員許可
	p := newOIDC(nil, nil)
	if ok, _ := p.IsMember(ctx, nil, guest); !ok {
		t.Error("要件なしでは許可されるべき")
	}

	// 必要ロール保有 → 許可 / 未保有 → 拒否
	p = newOIDC([]string{"approved"}, nil)
	if ok, _ := p.IsMember(ctx, nil, approved); !ok {
		t.Error("必要ロール保有なら許可されるべき")
	}
	if ok, _ := p.IsMember(ctx, nil, guest); ok {
		t.Error("必要ロール未保有なのに許可された")
	}

	// allowlist と併用 → 両方満たす必要がある（AND）
	p = newOIDC([]string{"approved"}, []string{"other.com"})
	if ok, _ := p.IsMember(ctx, nil, approved); ok {
		t.Error("ロールは満たすがメールドメインが不一致なのに許可された")
	}
}

// 継続確認: ロールを剥奪されたら既存セッションでも弾かれること。
// ログイン時だけ見ていると、剥奪後もセッションが生き続けてしまう。
func TestVerifyMembershipEnforcesRoleRevocation(t *testing.T) {
	roles := `["approved"]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"u1"},"roles":` + roles + `}`))
	}))
	defer srv.Close()

	p := NewDiscordProvider(DiscordConfig{BotToken: "tok", GuildID: "g", RequiredRoles: []string{"approved"}})
	p.apiBase = srv.URL

	// ロール保有中は通る
	ok, err := p.VerifyMembership(context.Background(), "u1")
	if err != nil || !ok {
		t.Fatalf("ロール保有中は在籍確認を通るべき: ok=%v err=%v", ok, err)
	}

	// ロールを剥奪（キャッシュも消して再取得させる）
	roles = `["member"]`
	p.ClearCache()

	ok, err = p.VerifyMembership(context.Background(), "u1")
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if ok {
		t.Error("ロール剥奪後も在籍確認を通っている（既存セッションが失効しない）")
	}
}
