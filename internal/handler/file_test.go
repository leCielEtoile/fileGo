package handler

import (
	"strings"
	"testing"
)

func TestContentDispositionAttachment(t *testing.T) {
	// クオートや制御文字を含む名前でも、ASCIIフォールバックが浄化され
	// filename* に元名がRFC5987で入ることを確認する。
	got := contentDispositionAttachment("evil\".txt")
	if strings.Contains(strings.SplitN(got, "filename*", 2)[0], "\"evil\".txt\"") {
		t.Errorf("ASCIIフォールバックにクオートが素通りしている: %q", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''") {
		t.Errorf("filename* が付与されていない: %q", got)
	}
}

func TestRFC5987Escape(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"simple.txt", "simple.txt"},
		{"a b.txt", "a%20b.txt"},
		{"q\"x", "q%22x"},
		{"日本語", "%E6%97%A5%E6%9C%AC%E8%AA%9E"},
	}
	for _, c := range cases {
		if got := rfc5987Escape(c.in); got != c.want {
			t.Errorf("rfc5987Escape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
