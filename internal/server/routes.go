package server

import (
	"encoding/json"
	"net/http"

	"github.com/aystro/apod/internal/engine"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	engine *engine.Engine
}

func NewHandler(e *engine.Engine) *Handler {
	return &Handler{engine: e}
}

type apiResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{OK: status < 400, Data: data})
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{OK: false, Error: msg})
}

func (h *Handler) CreateSite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string            `json:"domain"`
		Driver string            `json:"driver"`
		RAM    string            `json:"ram"`
		CPU    string            `json:"cpu"`
		Repo   string            `json:"repo"`
		Branch string            `json:"branch"`
		Params map[string]string `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Domain == "" || req.Driver == "" {
		respondError(w, http.StatusBadRequest, "domain and driver are required")
		return
	}

	err := h.engine.CreateSite(r.Context(), engine.CreateSiteOpts{
		Domain: req.Domain,
		Driver: req.Driver,
		RAM:    req.RAM,
		CPU:    req.CPU,
		Repo:   req.Repo,
		Branch: req.Branch,
		Params: req.Params,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	site, _ := h.engine.GetSite(r.Context(), req.Domain)
	respondJSON(w, http.StatusCreated, site)
}

func (h *Handler) ListSites(w http.ResponseWriter, r *http.Request) {
	sites, err := h.engine.ListSites(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, sites)
}

func (h *Handler) GetSite(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	site, err := h.engine.GetSite(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, site)
}

func (h *Handler) StartSite(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.StartSite(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) StopSite(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.StopSite(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) RestartSite(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.RestartSite(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *Handler) DestroySite(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	purge := r.URL.Query().Get("purge") == "true"

	if err := h.engine.DestroySite(r.Context(), domain, purge); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

func (h *Handler) ListDrivers(w http.ResponseWriter, r *http.Request) {
	drivers, err := h.engine.ListDrivers()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, drivers)
}
