//go:build dockerintegration

package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestIntegrationExecWithInputLargePayload proves the restore mechanism handles
// payloads larger than the shell argv limit (MAX_ARG_STRLEN, ~128 KB): streaming
// via stdin works, while embedding the payload in the command — the old, broken
// approach — fails. This is the regression guard for the DB-restore-silently-
// loses-data bug, which hid because earlier tests used tiny data.
func TestIntegrationExecWithInputLargePayload(t *testing.T) {
	e := newDockerEngine(t)
	ctx := context.Background()

	const name = "apod-itexec-test"
	cleanup := func() { e.docker.StopContainer(ctx, name); e.docker.RemoveContainer(ctx, name) }
	cleanup()
	t.Cleanup(cleanup)

	if err := e.docker.PullImage(ctx, itImage); err != nil {
		t.Fatalf("pull: %v", err)
	}
	id, err := e.docker.CreateContainer(ctx, ContainerConfig{
		Name: name, Image: itImage, Args: []string{"sleep", "3600"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := e.docker.StartContainer(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 1 MB payload — well over the ~128 KB argv limit.
	payload := bytes.Repeat([]byte("A"), 1<<20)

	// Streaming via stdin must succeed and deliver every byte.
	out, err := e.docker.ExecWithInput(ctx, name, []string{"sh", "-c", "wc -c"}, payload)
	if err != nil {
		t.Fatalf("ExecWithInput failed for %d-byte payload: %v", len(payload), err)
	}
	if !strings.Contains(out, fmt.Sprintf("%d", len(payload))) {
		t.Errorf("stdin payload truncated: wc -c = %q, want %d", strings.TrimSpace(out), len(payload))
	}

	// Embedding the same payload in the command argv must fail — demonstrating
	// why the old base64-in-argv restore broke for real databases.
	_, embErr := e.docker.ExecInContainer(ctx, name,
		[]string{"sh", "-c", "true " + string(payload)})
	if embErr == nil {
		t.Error("embedding a 1MB payload in argv unexpectedly succeeded; argv-limit assumption changed")
	}
}
