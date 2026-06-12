package server

import (
	"encoding/json"
	"net/http"
)

// SetupStatusHandler reports whether the instance still needs first-run setup.
// GET /api/v1/setup/status — unauthenticated (reveals only existence of users).
func (h *Handler) SetupStatusHandler(w http.ResponseWriter, r *http.Request) {
	needs, err := h.engine.NeedsSetup()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "setup status unavailable")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"needs_setup": needs})
}

// SetupHandler creates the first admin user. Unauthenticated, but only works
// while the instance has zero users — it self-disables afterward.
// POST /api/v1/setup { "name": "...", "password": "..." }
func (h *Handler) SetupHandler(w http.ResponseWriter, r *http.Request) {
	needs, err := h.engine.NeedsSetup()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "setup unavailable")
		return
	}
	if !needs {
		respondError(w, http.StatusForbidden, "setup already completed")
		return
	}

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

	user, err := h.engine.SetUpFirstAdmin(r.Context(), req.Name, req.Password)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{
		"name": user.Name,
		"role": user.Role,
	})
}
