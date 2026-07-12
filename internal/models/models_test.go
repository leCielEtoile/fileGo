package models

import "testing"

func TestSanitizeDirName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"通常のユーザー名は変化しない", "alice", "alice"},
		{"ドット入りの名前は保持する", "alice.smith", "alice.smith"},
		{"パス区切りは最終要素だけ残す", "a/b", "b"},
		{"親への相対パスは末尾要素に丸められる", "../../etc/passwd", "passwd"},
		{"連続ドットは無害化される", "..evil", "_evil"},
		{"バックスラッシュは置換される", "a\\b", "a_b"},
		{"前後の空白は除去される", "  bob  ", "bob"},
		{"単独ドットはプレースホルダになる", ".", "_"},
		{"空文字はプレースホルダになる", "", "_"},
		{"親ディレクトリ表現はプレースホルダになる", "..", "_"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SanitizeDirName(c.in); got != c.want {
				t.Errorf("SanitizeDirName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeDirNameIsIdempotent(t *testing.T) {
	for _, in := range []string{"alice", "../../x", "..evil", "a/b/c", "."} {
		once := SanitizeDirName(in)
		twice := SanitizeDirName(once)
		if once != twice {
			t.Errorf("非冪等: SanitizeDirName(%q)=%q, 2回目=%q", in, once, twice)
		}
	}
}
