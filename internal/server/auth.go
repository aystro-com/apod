package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aystro/apod/internal/engine"
	"github.com/go-chi/chi/v5"
)

// AuthLoginHandler exchanges a username/password for a session token.
// POST /api/v1/auth/login { "name": "...", "password": "..." }
// Unauthenticated by design; rate-limited tighter than the rest of the API.
func (h *Handler) AuthLoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "name and password are required")
		return
	}

	token, user, err := h.engine.LoginWithPassword(req.Name, req.Password, req.Code)
	if err != nil {
		// Distinguish "password OK, need a 2FA code" from a credential failure
		// so the UI can prompt for the code. The marker is not sensitive — it
		// only leaks that 2FA is on for a correct password.
		if errors.Is(err, engine.ErrTwoFactorRequired) {
			respondError(w, http.StatusUnauthorized, "2fa_required")
			return
		}
		// Locked after repeated failures. The same response is returned for
		// real and bogus usernames, so it does not enumerate accounts.
		if errors.Is(err, engine.ErrAccountLocked) {
			respondError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
			return
		}
		// Otherwise always the same generic message — no username enumeration.
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
	_, totpEnabled, _ := h.engine.GetUserTOTPStatus(user.Name)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":         user.Name,
		"role":         user.Role,
		"totp_enabled": totpEnabled,
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

// --- Two-factor authentication (session/self only) ---

func currentUserOrUnauth(w http.ResponseWriter, r *http.Request) string {
	user := UserFromContext(r.Context())
	if user == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return ""
	}
	return user.Name
}

// TwoFactorSetupHandler starts 2FA enrollment, returning a secret + otpauth URI.
// POST /api/v1/auth/2fa/setup
func (h *Handler) TwoFactorSetupHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	secret, uri, err := h.engine.Setup2FA(name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

// TwoFactorEnableHandler confirms enrollment with a code and returns recovery codes.
// POST /api/v1/auth/2fa/enable { "code": "123456" }
func (h *Handler) TwoFactorEnableHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	recovery, err := h.engine.Enable2FA(name, req.Code)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"recovery_codes": recovery})
}

// TwoFactorDisableHandler turns off 2FA after verifying a code.
// POST /api/v1/auth/2fa/disable { "code": "123456" }
func (h *Handler) TwoFactorDisableHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.Disable2FA(name, req.Code); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "2fa_disabled"})
}
