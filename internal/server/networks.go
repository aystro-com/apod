package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ownsNetwork reports whether the caller may manage a shared network (its owner,
// or an admin). Writes the error response and returns false otherwise.
func (h *Handler) ownsNetwork(w http.ResponseWriter, r *http.Request, name string) bool {
	user := UserFromContext(r.Context())
	if user == nil || user.Role == "admin" {
		return true
	}
	sn, ok, err := h.engine.GetSharedNetwork(r.Context(), name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if !ok {
		respondError(w, http.StatusNotFound, "network not found")
		return false
	}
	if sn.Owner != user.Name {
		respondError(w, http.StatusForbidden, "access denied")
		return false
	}
	return true
}

func (h *Handler) CreateNetworkHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Non-admins own the networks they create; admin-created networks are global.
	owner := ""
	if user := UserFromContext(r.Context()); user != nil && user.Role != "admin" {
		owner = user.Name
	}
	if err := h.engine.CreateSharedNetwork(r.Context(), req.Name, owner); err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

func (h *Handler) ListNetworksHandler(w http.ResponseWriter, r *http.Request) {
	// Admins see all networks; users see only their own.
	owner := ""
	if user := UserFromContext(r.Context()); user != nil && user.Role != "admin" {
		owner = user.Name
	}
	nets, err := h.engine.ListSharedNetworks(r.Context(), owner)
	if err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, nets)
}

func (h *Handler) DeleteNetworkHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !h.ownsNetwork(w, r, name) {
		return
	}
	if err := h.engine.DeleteSharedNetwork(r.Context(), name); err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) AddNetworkMemberHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The caller must own BOTH the network and the site being added — that's the
	// same-owner gate that keeps one tenant from wiring another's site into a
	// network. checkSiteAccess validates the domain + site ownership.
	if !h.ownsNetwork(w, r, name) || !h.checkSiteAccess(w, r, req.Domain) {
		return
	}
	if err := h.engine.AddSiteToNetwork(r.Context(), name, req.Domain); err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}

func (h *Handler) RemoveNetworkMemberHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	domain := chi.URLParam(r, "domain")
	if !isValidDomain(domain) {
		respondError(w, http.StatusBadRequest, "invalid domain")
		return
	}
	if !h.ownsNetwork(w, r, name) {
		return
	}
	if err := h.engine.RemoveSiteFromNetwork(r.Context(), name, domain); err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

// SiteNetworkHandler returns the neighbor containers a site can reach over its
// shared networks (for the architecture view).
func (h *Handler) SiteNetworkHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if !h.checkSiteAccess(w, r, domain) {
		return
	}
	neighbors, err := h.engine.SiteNetworkView(r.Context(), domain)
	if err != nil {
		respondEngineError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, neighbors)
}
