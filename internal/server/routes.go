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

func (h *Handler) AddDomain(w http.ResponseWriter, r *http.Request) {
	siteDomain := chi.URLParam(r, "domain")
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Domain == "" {
		respondError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if err := h.engine.AddDomain(r.Context(), siteDomain, req.Domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "added", "domain": req.Domain})
}

func (h *Handler) RemoveDomain(w http.ResponseWriter, r *http.Request) {
	siteDomain := chi.URLParam(r, "domain")
	removeDomain := chi.URLParam(r, "aliasDomain")
	if err := h.engine.RemoveDomain(r.Context(), siteDomain, removeDomain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed", "domain": removeDomain})
}

func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	siteDomain := chi.URLParam(r, "domain")
	domains, err := h.engine.ListDomains(r.Context(), siteDomain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, domains)
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	config, err := h.engine.GetConfig(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, config)
}

func (h *Handler) SetConfig(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if err := h.engine.SetConfig(r.Context(), domain, req.Key, req.Value); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) SetEnv(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if err := h.engine.SetEnv(r.Context(), domain, req.Key, req.Value); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "set", "key": req.Key})
}

func (h *Handler) UnsetEnv(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	key := chi.URLParam(r, "key")
	if err := h.engine.UnsetEnv(r.Context(), domain, key); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed", "key": key})
}

func (h *Handler) ListEnv(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	envs, err := h.engine.ListEnv(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, envs)
}

func (h *Handler) CreateBackupHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Storage string `json:"storage"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	id, err := h.engine.CreateBackup(r.Context(), domain, req.Storage)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]int64{"backup_id": id})
}

func (h *Handler) ListBackupsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	backups, err := h.engine.ListBackups(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, backups)
}

func (h *Handler) RestoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		BackupID int64 `json:"backup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.RestoreBackup(r.Context(), domain, req.BackupID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

func (h *Handler) DeleteBackupHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		BackupID int64 `json:"backup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.DeleteBackup(r.Context(), domain, req.BackupID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) AddBackupScheduleHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Every   string `json:"every"`
		Storage string `json:"storage"`
		Keep    int    `json:"keep"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Keep == 0 {
		req.Keep = 7
	}
	id, err := h.engine.AddBackupSchedule(r.Context(), domain, req.Every, req.Storage, req.Keep)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]int64{"schedule_id": id})
}

func (h *Handler) ListBackupSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	schedules, err := h.engine.ListBackupSchedules(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, schedules)
}

func (h *Handler) RemoveBackupScheduleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScheduleID int64 `json:"schedule_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.RemoveBackupSchedule(r.Context(), req.ScheduleID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) AddStorageConfigHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string            `json:"name"`
		Driver string            `json:"driver"`
		Config map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Driver == "" {
		respondError(w, http.StatusBadRequest, "name and driver are required")
		return
	}
	configJSON, _ := json.Marshal(req.Config)
	if err := h.engine.AddStorageConfig(req.Name, req.Driver, string(configJSON)); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "created", "name": req.Name})
}

func (h *Handler) ListStorageConfigsHandler(w http.ResponseWriter, r *http.Request) {
	configs, err := h.engine.ListStorageConfigs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, configs)
}

func (h *Handler) RemoveStorageConfigHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.engine.RemoveStorageConfig(name); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) DeployHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Branch string `json:"branch"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.Deploy(r.Context(), domain, req.Branch); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
}

func (h *Handler) RollbackHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.Rollback(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "rolled_back"})
}

func (h *Handler) ListDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	deps, err := h.engine.ListDeployments(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, deps)
}

func (h *Handler) CreateWebhookHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	token, err := h.engine.CreateWebhook(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"token": token, "url": "/webhook/" + token})
}

func (h *Handler) ListWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	whs, err := h.engine.ListWebhooks(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, whs)
}

func (h *Handler) DeleteWebhookHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.DeleteWebhook(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) IncomingWebhookHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := h.engine.HandleWebhook(r.Context(), token); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deploying"})
}

func (h *Handler) SiteLogsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	logs, err := h.engine.GetLogs(r.Context(), domain, 50)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, logs)
}

func (h *Handler) AllLogsHandler(w http.ResponseWriter, r *http.Request) {
	logs, err := h.engine.GetAllLogs(r.Context(), 100)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, logs)
}
