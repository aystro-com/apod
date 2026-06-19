package engine

import (
	"testing"
)

func TestProgressHubReplayAndLive(t *testing.T) {
	h := newProgressHub()
	h.Begin("ex.com")
	h.Emit("ex.com", ProgressEvent{Step: "Preparing", Percent: 5})
	h.Emit("ex.com", ProgressEvent{Step: "Pulling", Percent: 40})

	// A subscriber that connects mid-deploy replays what it missed…
	replay, ch, cancel := h.Subscribe("ex.com")
	defer cancel()
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want 2", len(replay))
	}
	if replay[0].Step != "Preparing" || replay[1].Step != "Pulling" {
		t.Errorf("replay order wrong: %+v", replay)
	}

	// …and receives subsequent live events.
	h.Emit("ex.com", ProgressEvent{Step: "Ready", Status: "done", Percent: 100})
	got := <-ch
	if got.Step != "Ready" || !got.Terminal() {
		t.Errorf("live event = %+v, want terminal Ready", got)
	}
}

func TestProgressHubBeginClearsBuffer(t *testing.T) {
	h := newProgressHub()
	h.Emit("ex.com", ProgressEvent{Step: "old"})
	h.Begin("ex.com") // a new deploy wipes the previous run's events
	replay, _, cancel := h.Subscribe("ex.com")
	defer cancel()
	if len(replay) != 0 {
		t.Errorf("Begin should clear the buffer, got %d events", len(replay))
	}
}

func TestProgressHubForgetFreesMemory(t *testing.T) {
	h := newProgressHub()
	h.Emit("ex.com", ProgressEvent{Step: "Ready", Status: "done", Percent: 100})

	// The retention cleanup frees the buffer so the hub doesn't grow per-site.
	h.forget("ex.com", h.gen["ex.com"])

	replay, _, cancel := h.Subscribe("ex.com")
	defer cancel()
	if len(replay) != 0 {
		t.Errorf("forget should free the buffer, got %d events", len(replay))
	}

	h.mu.Lock()
	_, hasBuf := h.buffers["ex.com"]
	h.mu.Unlock()
	if hasBuf {
		t.Error("buffer entry should be deleted after forget")
	}
}

// Emit concurrently with a subscriber cancelling must never panic with "send
// on closed channel" — the exact crash a client closing its tab mid-deploy
// used to trigger. Run with -race for full coverage.
func TestProgressHubEmitVsCancelNoPanic(t *testing.T) {
	h := newProgressHub()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			h.Emit("ex.com", ProgressEvent{Step: "x", Percent: i % 100})
		}
		close(done)
	}()
	for i := 0; i < 2000; i++ {
		_, _, cancel := h.Subscribe("ex.com")
		cancel()
	}
	<-done
}

// A retention timer from a finished run must not delete the buffer of a newer
// run that began within the retention window.
func TestProgressHubStaleForgetSparesFreshRun(t *testing.T) {
	h := newProgressHub()
	h.Emit("ex.com", ProgressEvent{Step: "old", Status: "done", Percent: 100})
	staleGen := h.gen["ex.com"] // the run the (pending) timer was scheduled for

	h.Begin("ex.com") // a new run starts, bumping the generation
	h.Emit("ex.com", ProgressEvent{Step: "fresh", Percent: 10})

	h.forget("ex.com", staleGen) // the stale timer fires

	replay, _, cancel := h.Subscribe("ex.com")
	defer cancel()
	if len(replay) == 0 || replay[len(replay)-1].Step != "fresh" {
		t.Errorf("stale forget wiped the fresh run's buffer: %+v", replay)
	}
}

func TestProgressHubCancelDropsEmptyDomain(t *testing.T) {
	h := newProgressHub()
	_, _, cancel := h.Subscribe("ex.com")
	cancel()
	h.mu.Lock()
	_, has := h.subs["ex.com"]
	h.mu.Unlock()
	if has {
		t.Error("empty subscriber set should be removed on cancel")
	}
}

func TestProgressHubIsolatesDomains(t *testing.T) {
	h := newProgressHub()
	h.Emit("a.com", ProgressEvent{Step: "a"})
	h.Emit("b.com", ProgressEvent{Step: "b"})
	replay, _, cancel := h.Subscribe("a.com")
	defer cancel()
	if len(replay) != 1 || replay[0].Step != "a" {
		t.Errorf("domain a should only see its own events, got %+v", replay)
	}
}

func TestProgressEmitNeverBlocks(t *testing.T) {
	h := newProgressHub()
	// Subscribe but never drain — Emit must not block the deploy.
	_, _, cancel := h.Subscribe("ex.com")
	defer cancel()
	for i := 0; i < 1000; i++ {
		h.Emit("ex.com", ProgressEvent{Step: "spam", Percent: i % 100})
	}
	// Reaching here without deadlock is the assertion.
}

func TestEngineEmitProgressLazyInit(t *testing.T) {
	// An engine built without the hub (e.g. NewWithDB / test literal) must still
	// emit safely and be subscribable.
	e := &Engine{}
	e.emitProgress("ex.com", "Step", "running", "", 10)
	replay, _, cancel := e.SubscribeProgress("ex.com")
	defer cancel()
	if len(replay) != 1 {
		t.Errorf("lazy-init hub should have buffered the event, got %d", len(replay))
	}
}

func TestSanitizeProgressLine(t *testing.T) {
	// Control characters (CR, ANSI) are stripped; length is capped.
	in := "\x1b[32m Container sonarr  Started \r"
	out := sanitizeProgressLine(in)
	if out != "[32m Container sonarr  Started" && out != "[32m Container sonarr  Started " {
		// ANSI '[' survives but the ESC (0x1b) and CR are gone; assert no control bytes.
	}
	for _, r := range out {
		if r < 0x20 || r == 0x7f {
			t.Errorf("control char survived sanitize: %q", out)
		}
	}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	if len(sanitizeProgressLine(string(long))) > 160 {
		t.Error("length not capped")
	}
}
