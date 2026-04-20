package engine

import "testing"

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	// Empty dir should be 0
	size := dirSize(dir)
	if size != 0 {
		t.Errorf("empty dir size = %d, want 0", size)
	}
}

func TestSplitLines(t *testing.T) {
	lines := splitLines("a\nb\nc")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3", len(lines))
	}
}
