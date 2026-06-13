package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/aystro/apod/internal/engine"
)

// TokenInfoFromContext returns the scoped-token grants for the request, or nil
// for full credentials (Unix socket, session token, or plain API key).
func TokenInfoFromContext(ctx context.Context) *engine.TokenInfo {
	t, _ := ctx.Value(ctxTokenKey).(*engine.TokenInfo)
	return t
}

// Route policy for scoped personal access tokens. Full credentials bypass all
// of this; only PATs are constrained. The policy is classified from the real
// request path (not chi's RoutePattern, which isn't resolved when group-level
// middleware runs), keeping the security boundary in one auditable function.

var deployActions = map[string]bool{
	"deploy": true, "rollback": true, "restart": true, "start": true, "stop": true,
}

// sensitiveLeaf maps the action segment of /sites/{domain}/<action> routes
// that expose or accept secrets (env values, DB creds, webhook tokens).
var sensitiveLeaf = map[string]bool{
	"env": true, "info": true, "webhook": true, "ftp": true, "db": true,
}

// patPolicy is the decision for a scoped token on a given request.
type patPolicy struct {
	management bool // mints credentials / changes auth — PATs forbidden outright
	deploy     bool // mutating method may be satisfied by the deploy ability
	sensitive  bool // requires the token's sensitive flag
}

// classify derives the policy from the path segments after /api/v1/.
func classify(method string, segs []string) patPolicy {
	var p patPolicy
	if len(segs) == 0 {
		return p
	}
	switch segs[0] {
	case "tokens":
		p.management = true
	case "auth":
		if len(segs) >= 2 && segs[1] == "2fa" {
			p.management = true
		}
	case "users":
		// /users/{name}/password, /users/{name}/reset-key, DELETE /users/{name}
		if len(segs) >= 3 && (segs[2] == "password" || segs[2] == "reset-key") {
			p.management = true
		}
		if len(segs) == 2 && method == http.MethodDelete {
			p.management = true
		}
	case "sites":
		// /sites/{domain}/<action> ... and /sites/{domain}/db/<export|import>
		if len(segs) >= 3 {
			action := segs[2]
			if deployActions[action] {
				p.deploy = true
			}
			if sensitiveLeaf[action] {
				p.sensitive = true
			}
		}
	}
	return p
}

// AbilityMiddleware enforces PAT scope. Full credentials pass through; scoped
// tokens are checked against the classified route policy.
func AbilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := TokenInfoFromContext(r.Context())
		if info == nil {
			next.ServeHTTP(w, r) // full credential — unrestricted
			return
		}

		path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1"), "/")
		segs := strings.Split(path, "/")
		p := classify(r.Method, segs)

		if p.management {
			respondError(w, http.StatusForbidden,
				"this action requires a session or API key, not a scoped token")
			return
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			if !info.HasAbility("read") {
				respondError(w, http.StatusForbidden, "token lacks the 'read' ability")
				return
			}
		default:
			allowed := info.HasAbility("write") || (p.deploy && info.HasAbility("deploy"))
			if !allowed {
				respondError(w, http.StatusForbidden,
					"token lacks the required ability for this action")
				return
			}
		}

		if p.sensitive && !info.Sensitive {
			respondError(w, http.StatusForbidden,
				"token is not permitted to access sensitive data")
			return
		}

		next.ServeHTTP(w, r)
	})
}
