package permission

import "testing"

func TestReadFilterCanRead(t *testing.T) {
	f := &ReadFilter{dirs: map[string]bool{"public": true, "user/alice": true}}

	cases := []struct {
		dir  string
		want bool
	}{
		{"public", true},
		{"public/sub", true},
		{"user/alice", true},
		{"user/alice/photos", true},
		{"user/bob", false},
		{"user/alicex", false},
		{"admin", false},
		{"publicx", false},
	}
	for _, c := range cases {
		if got := f.CanRead(c.dir); got != c.want {
			t.Errorf("CanRead(%q) = %v, want %v", c.dir, got, c.want)
		}
	}
}

func TestReadFilterAdminSeesEverything(t *testing.T) {
	f := &ReadFilter{admin: true}
	for _, dir := range []string{"public", "user/bob", "admin", "anything/deep"} {
		if !f.CanRead(dir) {
			t.Errorf("admin は %q を読めるべき", dir)
		}
	}
}

func TestReadFilterNilFailsClosed(t *testing.T) {
	var f *ReadFilter
	if f.CanRead("public") {
		t.Error("未解決(nil)のフィルタは全拒否であるべき")
	}
}
