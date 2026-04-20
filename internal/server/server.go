package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aystro/apod/internal/engine"
	"github.com/go-chi/chi/v5"
)

const defaultSocketPath = "/var/run/apod.sock"

type Server struct {
	handler *Handler
	router  *chi.Mux
}

func New(e *engine.Engine) *Server {
	h := NewHandler(e)
	r := chi.NewRouter()

	r.Use(RecoveryMiddleware)
	r.Use(LoggingMiddleware)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/sites", h.CreateSite)
		r.Get("/sites", h.ListSites)
		r.Get("/sites/{domain}", h.GetSite)
		r.Post("/sites/{domain}/start", h.StartSite)
		r.Post("/sites/{domain}/stop", h.StopSite)
		r.Post("/sites/{domain}/restart", h.RestartSite)
		r.Delete("/sites/{domain}", h.DestroySite)

		r.Get("/drivers", h.ListDrivers)

		// Domain management
		r.Get("/sites/{domain}/domains", h.ListDomains)
		r.Post("/sites/{domain}/domains", h.AddDomain)
		r.Delete("/sites/{domain}/domains/{aliasDomain}", h.RemoveDomain)

		// Config management
		r.Get("/sites/{domain}/config", h.GetConfig)
		r.Post("/sites/{domain}/config", h.SetConfig)

		// Env management
		r.Get("/sites/{domain}/env", h.ListEnv)
		r.Post("/sites/{domain}/env", h.SetEnv)
		r.Delete("/sites/{domain}/env/{key}", h.UnsetEnv)

		// Backup management
		r.Post("/sites/{domain}/backups", h.CreateBackupHandler)
		r.Get("/sites/{domain}/backups", h.ListBackupsHandler)
		r.Post("/sites/{domain}/backups/restore", h.RestoreBackupHandler)
		r.Delete("/sites/{domain}/backups", h.DeleteBackupHandler)

		// Backup schedules
		r.Post("/sites/{domain}/backups/schedule", h.AddBackupScheduleHandler)
		r.Get("/sites/{domain}/backups/schedule", h.ListBackupSchedulesHandler)
		r.Delete("/sites/{domain}/backups/schedule", h.RemoveBackupScheduleHandler)

		// Storage configs
		r.Post("/storage", h.AddStorageConfigHandler)
		r.Get("/storage", h.ListStorageConfigsHandler)
		r.Delete("/storage/{name}", h.RemoveStorageConfigHandler)

		// Deploy
		r.Post("/sites/{domain}/deploy", h.DeployHandler)
		r.Post("/sites/{domain}/rollback", h.RollbackHandler)
		r.Get("/sites/{domain}/deployments", h.ListDeploymentsHandler)

		// Webhooks
		r.Post("/sites/{domain}/webhook", h.CreateWebhookHandler)
		r.Get("/sites/{domain}/webhook", h.ListWebhooksHandler)
		r.Delete("/sites/{domain}/webhook", h.DeleteWebhookHandler)

		// Logs
		r.Get("/sites/{domain}/logs", h.SiteLogsHandler)
		r.Get("/logs", h.AllLogsHandler)

		// Monitoring
		r.Get("/sites/{domain}/monitor", h.MonitorSiteHandler)
		r.Get("/monitor", h.MonitorAllHandler)

		// Uptime
		r.Post("/sites/{domain}/uptime", h.EnableUptimeHandler)
		r.Get("/sites/{domain}/uptime", h.UptimeStatusHandler)
		r.Delete("/sites/{domain}/uptime", h.DisableUptimeHandler)
		r.Get("/sites/{domain}/uptime/logs", h.UptimeLogsHandler)

		// Container logs
		r.Get("/sites/{domain}/container-logs", h.ContainerLogsHandler)

		// Clone
		r.Post("/sites/{domain}/clone", h.CloneSiteHandler)

		// Database
		r.Get("/sites/{domain}/db/export", h.DBExportHandler)
		r.Post("/sites/{domain}/db/import", h.DBImportHandler)

		// Server stats
		r.Get("/server-stats", h.ServerStatsHandler)
		r.Get("/disk-usage", h.DiskUsageHandler)

		// Cron jobs
		r.Post("/sites/{domain}/cron", h.AddCronJobHandler)
		r.Get("/sites/{domain}/cron", h.ListCronJobsHandler)
		r.Delete("/sites/{domain}/cron", h.RemoveCronJobHandler)

		// Proxy rules
		r.Post("/sites/{domain}/proxy", h.AddProxyRuleHandler)
		r.Get("/sites/{domain}/proxy", h.ListProxyRulesHandler)
		r.Delete("/sites/{domain}/proxy", h.RemoveProxyRuleHandler)

		// IP blocking
		r.Post("/sites/{domain}/ip/block", h.BlockIPHandler)
		r.Post("/sites/{domain}/ip/unblock", h.UnblockIPHandler)
		r.Get("/sites/{domain}/ip", h.ListIPRulesHandler)

		// FTP
		r.Post("/sites/{domain}/ftp", h.AddFTPAccountHandler)
		r.Get("/sites/{domain}/ftp", h.ListFTPAccountsHandler)
		r.Delete("/sites/{domain}/ftp/{username}", h.RemoveFTPAccountHandler)

		// Firewall
		r.Get("/firewall", h.FirewallStatusHandler)
		r.Post("/firewall/enable", h.FirewallEnableHandler)
		r.Post("/firewall/allow", h.FirewallAllowHandler)
		r.Post("/firewall/deny", h.FirewallDenyHandler)

		// SSH Keys
		r.Post("/ssh-keys", h.AddSSHKeyHandler)
		r.Get("/ssh-keys", h.ListSSHKeysHandler)
		r.Delete("/ssh-keys/{name}", h.RemoveSSHKeyHandler)

		// System
		r.Get("/version", h.VersionHandler)
		r.Get("/update/check", h.CheckUpdateHandler)
		r.Post("/update", h.UpdateHandler)
		r.Post("/update/drivers", h.UpdateDriversHandler)
	})

	r.Post("/webhook/{token}", h.IncomingWebhookHandler)

	return &Server{handler: h, router: r}
}

func (s *Server) ListenSocket(socketPath string) error {
	if socketPath == "" {
		socketPath = defaultSocketPath
	}

	os.Remove(socketPath)

	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}

	os.Chmod(socketPath, 0660)

	log.Printf("apod daemon listening on %s", socketPath)
	return http.Serve(listener, s.router)
}

func (s *Server) ListenTCP(addr string) error {
	log.Printf("apod daemon listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) Shutdown(ctx context.Context) error {
	os.Remove(defaultSocketPath)
	return nil
}
