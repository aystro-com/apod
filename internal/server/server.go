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
