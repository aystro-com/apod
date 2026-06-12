package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// AuthLoginHandler exchanges a username/password for a session token.
// POST /api/v1/auth/login { "name": "...", "password": "..." }
// Unauthenticated by design; rate-limited tighter than the rest of the API.
func (h *Handler) AuthLoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "name and password are required")
		return
	}

	token, user, err := h.engine.LoginWithPassword(req.Name, req.Password)
	if err != nil {
		// Always the same generic message — no username enumeration.
		respondError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user": map[string]string{
			"name": user.Name,
			"role": user.Role,
		},
	})
}

// AuthMeHandler returns the authenticated identity — works with both API
// keys and session tokens. GET /api/v1/auth/me
func (h *Handler) AuthMeHandler(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"name": user.Name,
		"role": user.Role,
	})
}

// AuthLogoutHandler revokes the current session token.
// POST /api/v1/auth/logout — a no-op for API keys (they don't expire).
func (h *Handler) AuthLogoutHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := h.engine.Logout(token); err != nil {
		respondError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// SetUserPasswordHandler sets a login password.
// POST /api/v1/users/{name}/password { "password": "..." }
// Admins can set anyone's password; users only their own.
func (h *Handler) SetUserPasswordHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	current := UserFromContext(r.Context())
	if current == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if current.Role != "admin" && current.Name != name {
		respondError(w, http.StatusForbidden, "you can only change your own password")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.SetUserPassword(name, req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "password_set"})
}
