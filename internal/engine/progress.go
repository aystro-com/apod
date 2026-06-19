package engine

import (
	"sync"
	"time"
)

// ProgressEvent is a single step in a site deployment, streamed to the UI so it
// can show a live "what's being deployed" view instead of an opaque spinner.
type ProgressEvent struct {
	Step    string    `json:"step"`             // short phase label, e.g. "Pulling images"
	Status  string    `json:"status"`           // "running" | "done" | "error"
	Detail  string    `json:"detail,omitempty"` // optional line of detail (a service, an image, …)
	Percent int       `json:"percent"`          // overall completion, 0-100
	Time    time.Time `json:"time"`
}

// Terminal reports whether this event ends the deployment stream (success or
// failure), so a subscriber knows to stop.
func (e ProgressEvent) Terminal() bool {
	return e.Status == "error" || e.Percent >= 100
}

// progressRetention is how long a finished deployment's events are kept so a
// page reload can still replay them, after which the buffer is freed.
const progressRetention = 2 * time.Minute

// progressHub is an in-memory pub/sub of deployment progress, keyed by domain.
// It buffers events so a subscriber that connects mid-deploy (or just after it
// finishes) still replays the full sequence, then receives live updates.
// Buffers are freed shortly after a deploy ends so memory does not grow with
// the number of sites ever deployed.
type progressHub struct {
	mu      sync.Mutex
	buffers map[string][]ProgressEvent
	subs    map[string]map[chan ProgressEvent]struct{}
}

func newProgressHub() *progressHub {
	return &progressHub{
		buffers: map[string][]ProgressEvent{},
		subs:    map[string]map[chan ProgressEvent]struct{}{},
	}
}

// Begin clears any prior buffer for a domain — a fresh deployment is starting.
func (h *progressHub) Begin(domain string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.buffers, domain)
}

// Emit records an event and fans it out to current subscribers.
func (h *progressHub) Emit(domain string, ev ProgressEvent) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.Status == "" {
		ev.Status = "running"
	}

	h.mu.Lock()
	buf := append(h.buffers[domain], ev)
	if len(buf) > 200 { // bound memory for pathological cases
		buf = buf[len(buf)-200:]
	}
	h.buffers[domain] = buf
	subs := make([]chan ProgressEvent, 0, len(h.subs[domain]))
	for ch := range h.subs[domain] {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default: // a slow consumer never blocks the deploy; it has the replay buffer
		}
	}

	// Once a deploy ends, free its buffer after a short retention window so the
	// hub does not accumulate one buffer per site forever.
	if ev.Terminal() {
		time.AfterFunc(progressRetention, func() { h.forget(domain) })
	}
}

// forget releases a domain's buffer (and any empty subscriber set) once its
// retention window has elapsed.
func (h *progressHub) forget(domain string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.buffers, domain)
	if len(h.subs[domain]) == 0 {
		delete(h.subs, domain)
	}
}

// Subscribe returns a replay of everything emitted so far for the domain, a
// channel of future events, and an unsubscribe function.
func (h *progressHub) Subscribe(domain string) (replay []ProgressEvent, ch <-chan ProgressEvent, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	replay = append([]ProgressEvent(nil), h.buffers[domain]...)
	c := make(chan ProgressEvent, 256)
	if h.subs[domain] == nil {
		h.subs[domain] = map[chan ProgressEvent]struct{}{}
	}
	h.subs[domain][c] = struct{}{}
	cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[domain][c]; ok {
			delete(h.subs[domain], c)
			close(c)
		}
		if len(h.subs[domain]) == 0 {
			delete(h.subs, domain) // don't keep empty per-domain maps around
		}
	}
	return replay, c, cancel
}

// --- Engine integration -----------------------------------------------------

// prog lazily creates the hub so it works regardless of which constructor built
// the engine (the real daemon, tests, etc.).
func (e *Engine) prog() *progressHub {
	e.progressOnce.Do(func() {
		if e.progress == nil {
			e.progress = newProgressHub()
		}
	})
	return e.progress
}

// beginDeploy resets progress for a domain and emits the first step.
func (e *Engine) beginDeploy(domain, step string) {
	e.prog().Begin(domain)
	e.emitProgress(domain, step, "running", "", 2)
}

// beginOp starts a fresh progress stream for any long-running operation (clone,
// destroy, backup, restore…). It's the non-deploy-specific name for the same
// mechanism, so every operation streams live progress the way a deploy does.
func (e *Engine) beginOp(domain, step string) {
	e.beginDeploy(domain, step)
}

// finishOp emits the terminal progress event for an operation: 100%/"done" on
// success, or an "error" carrying a sanitized first line of the failure. Pairs
// with beginOp so a subscriber always sees a clean end to the stream.
func (e *Engine) finishOp(domain, doneStep, doneDetail string, err error) {
	if err != nil {
		e.emitProgress(domain, "Failed", "error", sanitizeProgressLine(firstLine(err.Error())), 0)
		return
	}
	e.emitProgress(domain, doneStep, "done", doneDetail, 100)
}

// emitProgress publishes a deployment step for a domain.
func (e *Engine) emitProgress(domain, step, status, detail string, percent int) {
	e.prog().Emit(domain, ProgressEvent{
		Step:    step,
		Status:  status,
		Detail:  detail,
		Percent: percent,
	})
}

// SubscribeProgress exposes the hub to the HTTP layer for streaming.
func (e *Engine) SubscribeProgress(domain string) ([]ProgressEvent, <-chan ProgressEvent, func()) {
	return e.prog().Subscribe(domain)
}

// EmitProgress is the exported emitter (mirrors SubscribeProgress).
func (e *Engine) EmitProgress(domain string, ev ProgressEvent) {
	e.prog().Emit(domain, ev)
}
