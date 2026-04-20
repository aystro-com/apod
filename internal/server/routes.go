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

func (h *Handler) CloneSiteHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Target == "" {
		respondError(w, http.StatusBadRequest, "target domain is required")
		return
	}
	if err := h.engine.Clone(r.Context(), domain, req.Target); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "cloned", "target": req.Target})
}

func (h *Handler) DBExportHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	dump, err := h.engine.DBExport(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"dump": dump})
}

func (h *Handler) DBImportHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Dump string `json:"dump"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.DBImport(r.Context(), domain, req.Dump); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "imported"})
}

func (h *Handler) ServerStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := h.engine.GetServerStats(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

func (h *Handler) DiskUsageHandler(w http.ResponseWriter, r *http.Request) {
	usage, err := h.engine.GetDiskUsage(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, usage)
}

func (h *Handler) AddCronJobHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Schedule == "" || req.Command == "" {
		respondError(w, http.StatusBadRequest, "schedule and command are required")
		return
	}
	id, err := h.engine.AddCronJob(r.Context(), domain, req.Schedule, req.Command, req.Service)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]int64{"cron_id": id})
}

func (h *Handler) ListCronJobsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	jobs, err := h.engine.ListCronJobs(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, jobs)
}

func (h *Handler) RemoveCronJobHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.engine.RemoveCronJob(r.Context(), req.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) IncomingWebhookHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := h.engine.HandleWebhook(r.Context(), token); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deploying"})
}

func (h *Handler) MonitorSiteHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	stats, err := h.engine.GetSiteStats(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

func (h *Handler) MonitorAllHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := h.engine.GetAllStats(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

func (h *Handler) EnableUptimeHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		URL          string `json:"url"`
		Interval     int    `json:"interval"`
		AlertWebhook string `json:"alert_webhook"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return
	}
	if req.Interval == 0 {
		req.Interval = 60
	}
	if err := h.engine.EnableUptime(r.Context(), domain, req.URL, req.Interval, req.AlertWebhook); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "enabled"})
}

func (h *Handler) DisableUptimeHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	if err := h.engine.DisableUptime(r.Context(), domain); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (h *Handler) UptimeStatusHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	status, err := h.engine.GetUptimeStatus(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, status)
}

func (h *Handler) UptimeLogsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	logs, err := h.engine.GetUptimeLogs(r.Context(), domain, 50)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, logs)
}

func (h *Handler) ContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	lines := 100
	output, err := h.engine.GetContainerLogs(r.Context(), domain, lines)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"logs": output})
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

// Proxy rules
func (h *Handler) AddProxyRuleHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Type   string            `json:"type"`
		Config map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := h.engine.AddProxyRule(r.Context(), domain, req.Type, req.Config)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]int64{"rule_id": id})
}

func (h *Handler) ListProxyRulesHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	rules, err := h.engine.ListProxyRules(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, rules)
}

func (h *Handler) RemoveProxyRuleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.RemoveProxyRule(r.Context(), req.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// IP blocking
func (h *Handler) BlockIPHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		IP string `json:"ip"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.BlockIP(r.Context(), domain, req.IP); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "blocked", "ip": req.IP})
}

func (h *Handler) UnblockIPHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		IP string `json:"ip"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.UnblockIP(r.Context(), domain, req.IP); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "unblocked"})
}

func (h *Handler) ListIPRulesHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	rules, err := h.engine.ListIPRules(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, rules)
}

// FTP
func (h *Handler) AddFTPAccountHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.AddFTPAccount(r.Context(), domain, req.Username, req.Password); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "created", "username": req.Username})
}

func (h *Handler) ListFTPAccountsHandler(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	accounts, err := h.engine.ListFTPAccounts(r.Context(), domain)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, accounts)
}

func (h *Handler) RemoveFTPAccountHandler(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if err := h.engine.RemoveFTPAccount(r.Context(), username); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// Firewall
func (h *Handler) FirewallStatusHandler(w http.ResponseWriter, r *http.Request) {
	status, err := h.engine.FirewallStatus(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, status)
}

func (h *Handler) FirewallEnableHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.FirewallEnable(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

func (h *Handler) FirewallAllowHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port string `json:"port"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.FirewallAllow(r.Context(), req.Port); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "allowed", "port": req.Port})
}

func (h *Handler) FirewallDenyHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port string `json:"port"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.FirewallDeny(r.Context(), req.Port); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "denied", "port": req.Port})
}

// SSH Keys
func (h *Handler) AddSSHKeyHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := h.engine.AddSSHKey(r.Context(), req.Name, req.PublicKey); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "added", "name": req.Name})
}

func (h *Handler) ListSSHKeysHandler(w http.ResponseWriter, r *http.Request) {
	keys, err := h.engine.ListSSHKeys(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, keys)
}

func (h *Handler) RemoveSSHKeyHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.engine.RemoveSSHKey(r.Context(), name); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) VersionHandler(w http.ResponseWriter, r *http.Request) {
	version := h.engine.GetVersion()
	dbVersion := h.engine.GetDBVersion()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"version":    version,
		"db_version": dbVersion,
	})
}

func (h *Handler) CheckUpdateHandler(w http.ResponseWriter, r *http.Request) {
	latest, hasUpdate, err := h.engine.CheckForUpdate(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"current":    engine.Version,
		"latest":     latest,
		"has_update": hasUpdate,
	})
}

func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.SelfUpdate(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated", "message": "restart apod server to use new version"})
}

func (h *Handler) UpdateDriversHandler(w http.ResponseWriter, r *http.Request) {
	updated, err := h.engine.UpdateDrivers(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"updated": updated})
}
