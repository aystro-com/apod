package engine

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// chownDataOwner makes a service's bind-mount writable by the uid its image runs
// as. Verify it parses "uid:gid" / "uid" and applies recursively, and that it's
// a safe no-op for empty/garbage input.
func TestChownDataOwner(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("chown requires root")
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "data")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "f")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	chownDataOwner(dir, "101:102")

	for _, p := range []string{dir, sub, file} {
		var st syscall.Stat_t
		if err := syscall.Stat(p, &st); err != nil {
			t.Fatal(err)
		}
		if st.Uid != 101 || st.Gid != 102 {
			t.Errorf("%s: got uid=%d gid=%d, want 101/102", p, st.Uid, st.Gid)
		}
	}

	// "uid" alone applies the same id to gid.
	chownDataOwner(file, "55")
	var st syscall.Stat_t
	if err := syscall.Stat(file, &st); err != nil {
		t.Fatal(err)
	}
	if st.Uid != 55 || st.Gid != 55 {
		t.Errorf("single uid: got uid=%d gid=%d, want 55/55", st.Uid, st.Gid)
	}

	// Empty/garbage are no-ops (must not panic or change ownership).
	chownDataOwner(file, "")
	chownDataOwner(file, "not-a-number")
	if err := syscall.Stat(file, &st); err != nil {
		t.Fatal(err)
	}
	if st.Uid != 55 {
		t.Errorf("no-op changed owner to uid=%d", st.Uid)
	}
}
