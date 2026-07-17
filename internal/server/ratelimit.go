package server

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// trustedProxyNets are the peer networks whose X-Forwarded-For header we trust.
// Defaults to loopback (the typical same-host reverse-proxy case) and can be
// extended via APOD_TRUSTED_PROXIES (comma-separated CIDRs). Without this,
// X-Forwarded-For is attacker-controlled and trivially defeats the limiter.
var trustedProxyNets = loadTrustedProxies()

func loadTrustedProxies() []*net.IPNet {
	nets := []*net.IPNet{}
	for _, cidr := range []string{"127.0.0.0/8", "::1/128"} {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, n)
		}
	}
	for _, cidr := range strings.Split(os.Getenv("APOD_TRUSTED_PROXIES"), ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			// Allow a bare IP by treating it as a /32 or /128.
			if ip := net.ParseIP(cidr); ip != nil {
				if ip.To4() != nil {
					cidr += "/32"
				} else {
					cidr += "/128"
				}
			}
		}
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trustedProxyNets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// clientIP derives the rate-limit key. It only honors X-Forwarded-For when the
// direct peer is a trusted proxy, and then uses the right-most (proxy-appended)
// entry rather than the fully client-controlled left-most one.
func clientIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	// Honor X-Forwarded-For when the peer is a trusted proxy, or when the request
	// genuinely came through the bundled web proxy over the LOCAL socket. The
	// socket check is essential: X-Apod-Proxied is a plain header, so without it
	// a TCP client could set it plus a rotating X-Forwarded-For to mint a fresh
	// rate-limit key per request and defeat the brute-force limits.
	isUnix, _ := r.Context().Value(ctxIsUnixSocket).(bool)
	if isTrustedProxy(ip) || (isUnix && isProxiedWeb(r)) {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			candidate := strings.TrimSpace(parts[len(parts)-1])
			if candidate != "" {
				return candidate
			}
		}
	}
	return ip
}

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	// Periodic cleanup of stale entries
	go func() {
		for {
			time.Sleep(window)
			rl.mu.Lock()
			now := time.Now()
			for key, times := range rl.requests {
				var valid []time.Time
				for _, t := range times {
					if now.Sub(t) < window {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.requests, key)
				} else {
					rl.requests[key] = valid
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Filter to only recent requests
	var recent []time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		rl.requests[key] = recent
		return false
	}

	rl.requests[key] = append(recent, now)
	return true
}

// RateLimitMiddleware limits requests per IP address
func RateLimitMiddleware(limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := newRateLimiter(limit, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for direct Unix socket access (local CLI).
			// Requests forwarded by the web proxy are still rate limited so the
			// login/2FA endpoints remain brute-force protected.
			if isUnix, _ := r.Context().Value(ctxIsUnixSocket).(bool); isUnix && !isProxiedWeb(r) {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)

			if !limiter.allow(ip) {
				respondError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
