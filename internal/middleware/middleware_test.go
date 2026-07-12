package middleware

import (
	"net/http"
	"testing"
)

func TestClientIPFromXFF(t *testing.T) {
	trusted := []string{"10.0.0.0/8", "192.168.0.0/16"}

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"信頼プロキシ経由でXFF末尾のクライアントを採用", "10.0.0.1:1234", "203.0.113.5", "203.0.113.5"},
		{"多段の信頼プロキシを剥がして非信頼を採用", "10.0.0.1:1234", "203.0.113.5, 10.0.0.9, 192.168.1.1", "203.0.113.5"},
		{"直前ホップが非信頼ならXFFを無視", "203.0.113.9:1234", "203.0.113.5", ""},
		{"XFFが無ければ空", "10.0.0.1:1234", "", ""},
		{"全て信頼プロキシなら採用値なし", "10.0.0.1:1234", "10.0.0.2, 192.168.1.1", ""},
		{"クライアント詐称: 偽装した左端は末尾の非信頼が優先", "10.0.0.1:1234", "1.1.1.1, 203.0.113.5", "203.0.113.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Request{
				RemoteAddr: c.remoteAddr,
				Header:     http.Header{},
			}
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIPFromXFF(r, trusted); got != c.want {
				t.Errorf("clientIPFromXFF() = %q, want %q", got, c.want)
			}
		})
	}
}
