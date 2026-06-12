package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aystro/apod/internal/engine"
	"github.com/aystro/apod/internal/models"
)

type contextKey string

const (
	ctxUserKey      contextKey = "user"
	ctxIsUnixSocket contextKey = "unix_socket"
	ctxTokenKey     contextKey = "token_info"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// UnixSocketMiddleware marks requests as coming from the local Unix socket (admin access)
func UnixSocketMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxIsUnixSocket, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthMiddleware validates API keys for TCP connections
func AuthMiddleware(eng *engine.Engine) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Unix socket connections are always admin
			if isUnix, _ := r.Context().Value(ctxIsUnixSocket).(bool); isUnix {
				adminUser := &models.User{Name: "__admin__", Role: "admin"}
				ctx := context.WithValue(r.Context(), ctxUserKey, adminUser)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// TCP connections require API key
			auth := r.Header.Get("Authorization")
			if auth == "" {
				respondError(w, http.StatusUnauthorized, "API key required")
				return
			}

			key := strings.TrimPrefix(auth, "Bearer ")
			if key == auth {
				respondError(w, http.StatusUnauthorized, "invalid authorization format")
				return
			}

			// Three Bearer credential kinds share the same scheme, routed by
			// prefix: session tokens (web UI), scoped PATs (apod_pat_), and
			// long-lived API keys. Only PATs carry ability restrictions.
			var user *models.User
			var tokenInfo *engine.TokenInfo
			var err error
			switch {
			case engine.IsSessionToken(key):
				user, err = eng.ValidateSessionToken(key)
				if err != nil || user == nil {
					respondError(w, http.StatusUnauthorized, "invalid or expired session")
					return
				}
			case engine.IsAPIToken(key):
				user, tokenInfo, err = eng.ValidateAPIToken(key)
				if err != nil || user == nil || tokenInfo == nil {
					respondError(w, http.StatusUnauthorized, "invalid or expired token")
					return
				}
			default:
				user, err = eng.GetUserByAPIKeyHash(engine.HashAPIKey(key))
				if err != nil || user == nil {
					respondError(w, http.StatusUnauthorized, "invalid API key")
					return
				}
			}

			ctx := context.WithValue(r.Context(), ctxUserKey, user)
			if tokenInfo != nil {
				ctx = context.WithValue(ctx, ctxTokenKey, tokenInfo)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminOnlyMiddleware rejects non-admin users
func AdminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			respondError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFromContext extracts the authenticated user from context
func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(ctxUserKey).(*models.User)
	return u
}

// IsAdmin checks if the current user has admin role
func IsAdmin(ctx context.Context) bool {
	u := UserFromContext(ctx)
	return u != nil && u.Role == "admin"
}
