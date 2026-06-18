package server

import (
	"encoding/json"
	"net/http"
)

// CreateAPITokenHandler issues a scoped personal access token for the current
// user. POST /api/v1/tokens
// Reachable only by session/API-key auth (PATs cannot mint tokens — enforced
// by the route's ability gate requiring the special "manage" path).
func (h *Handler) CreateAPITokenHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	var req struct {
		Name      string   `json:"name"`
		Abilities []string `json:"abilities"`
		Sensitive bool     `json:"sensitive"`
		TTLDays   int      `json:"ttl_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	raw, err := h.engine.CreateAPIToken(name, req.Name, req.Abilities, req.Sensitive, req.TTLDays)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"token": raw})
}

// ListAPITokensHandler lists the current user's tokens (metadata only).
// GET /api/v1/tokens
func (h *Handler) ListAPITokensHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	tokens, err := h.engine.ListAPITokens(name)
	if err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"tokens": tokens})
}

// RevokeAPITokenHandler deletes one of the current user's tokens.
// DELETE /api/v1/tokens { "id": 1 }
func (h *Handler) RevokeAPITokenHandler(w http.ResponseWriter, r *http.Request) {
	name := currentUserOrUnauth(w, r)
	if name == "" {
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.RevokeAPIToken(name, req.ID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
