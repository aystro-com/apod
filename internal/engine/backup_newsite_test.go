package engine

import (
	"context"
	"testing"
)

func TestCreateSiteFromBackupGuards(t *testing.T) {
	e := newProcEngine(t)

	// Invalid new domain is rejected before any storage/Docker work.
	if err := e.CreateSiteFromBackup(context.Background(), "src.example.com", 1, "not a domain", ""); err == nil {
		t.Error("invalid new domain should be rejected")
	}

	// A valid domain but a non-existent backup id errors cleanly.
	if err := e.CreateSiteFromBackup(context.Background(), "src.example.com", 99999, "new.example.com", ""); err == nil {
		t.Error("missing backup should error")
	}
}
