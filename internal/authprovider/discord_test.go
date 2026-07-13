package authprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newStubProvider は Discord REST をスタブサーバーへ向けたProviderを返し、
// スタブが受けたリクエスト数を数えるカウンタを渡します。
func newStubProvider(t *testing.T, handler http.HandlerFunc) (*DiscordProvider, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	p := NewDiscordProvider(DiscordConfig{BotToken: "valid-looking-token", GuildID: "g1"})
	p.apiBase = srv.URL
	return p, &calls
}

// 401（トークン拒否）を受けたあと、以降のリクエストでDiscordを叩き続けないこと。
// これを怠るとDiscordの濫用検知でBotトークンが強制リセットされる。
func TestAuthFailureStopsCallingDiscord(t *testing.T) {
	p, calls := newStubProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	for i := 0; i < 50; i++ {
		if _, err := p.VerifyMembership(context.Background(), "u1"); err == nil {
			t.Fatal("401なのにエラーが返っていない")
		}
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("401後もDiscordを叩いている: %d回（クールダウンで1回に抑えるべき）", got)
	}
}

// 429（レート制限）でも同様に叩き続けないこと。429に429で応答し続けるのが最悪の増幅ループ。
func TestRateLimitStopsCallingDiscord(t *testing.T) {
	p, calls := newStubProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	for i := 0; i < 50; i++ {
		_, _ = p.VerifyMembership(context.Background(), "u1")
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("429後もDiscordを叩いている: %d回（Retry-Afterに従い1回に抑えるべき）", got)
	}
}

// ひな型トークン（YOUR_BOT_TOKEN）ではネットワークへ一切出ないこと。
// 公開イメージの初回起動で全ユーザーが無効な認証を撃つのを防ぐ。
func TestPlaceholderTokenNeverCallsDiscord(t *testing.T) {
	p, calls := newStubProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	p.botToken = "YOUR_BOT_TOKEN"

	for i := 0; i < 10; i++ {
		if _, err := p.VerifyMembership(context.Background(), "u1"); err == nil {
			t.Fatal("ひな型トークンなのにエラーが返っていない")
		}
	}

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("ひな型トークンでDiscordを叩いた: %d回（0であるべき）", got)
	}
}

// 成功したらクールダウンは解除され、キャッシュTTL内は再取得しないこと。
func TestSuccessCachesAndClearsCooldown(t *testing.T) {
	p, calls := newStubProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"u1","username":"alice"},"roles":["r1"]}`))
	})

	for i := 0; i < 5; i++ {
		ok, err := p.VerifyMembership(context.Background(), "u1")
		if err != nil || !ok {
			t.Fatalf("在籍確認に失敗: ok=%v err=%v", ok, err)
		}
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("キャッシュが効いていない: %d回（1回であるべき）", got)
	}
	if _, inCd := p.inCooldown(); inCd {
		t.Error("成功したのにクールダウンが残っている")
	}
}

func TestIsPlaceholderToken(t *testing.T) {
	cases := map[string]bool{
		"":                    true,
		"YOUR_BOT_TOKEN":      true,
		"YOUR_CLIENT_SECRET":  true,
		"MTIzNDU2Nzg5.abc.d":  false,
		"real-looking-token1": false,
	}
	for in, want := range cases {
		if got := isPlaceholderToken(in); got != want {
			t.Errorf("isPlaceholderToken(%q) = %v, want %v", in, got, want)
		}
	}
}

// 再接続しても回復し得ないcloseコードでは即座に諦めること
// （discordgo既定の無限再接続が濫用の主因のため）。
func TestFatalGatewayClose(t *testing.T) {
	fatal := []int{4004, 4010, 4011, 4012, 4013, 4014}
	for _, code := range fatal {
		err := &websocket.CloseError{Code: code, Text: "x"}
		if _, ok := fatalGatewayClose(err); !ok {
			t.Errorf("close %d は致命的として扱うべき", code)
		}
	}

	// 一時的な切断は再接続してよい
	transient := []int{1000, 1001, 1006, 4000, 4009}
	for _, code := range transient {
		err := &websocket.CloseError{Code: code, Text: "x"}
		if _, ok := fatalGatewayClose(err); ok {
			t.Errorf("close %d は一時的として再接続を許すべき", code)
		}
	}

	if _, ok := fatalGatewayClose(nil); ok {
		t.Error("nilを致命的と判定してはいけない")
	}
}

func TestRetryAfter(t *testing.T) {
	// ヘッダを優先
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"30"}}}
	if got := retryAfter(resp, nil); got != 30*time.Second {
		t.Errorf("ヘッダ優先が効いていない: %v", got)
	}

	// ヘッダが無ければボディの retry_after
	resp = &http.Response{Header: http.Header{}}
	if got := retryAfter(resp, []byte(`{"retry_after":2.5}`)); got != 2500*time.Millisecond {
		t.Errorf("ボディのretry_afterを読めていない: %v", got)
	}

	// どちらも無ければ既定値（0秒即再試行を避ける）
	if got := retryAfter(resp, []byte(`{}`)); got != discordDefaultRetryAfter {
		t.Errorf("既定値が使われていない: %v", got)
	}
}
