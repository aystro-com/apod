package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aystro/apod/internal/engine"
	"github.com/go-chi/chi/v5"
)

// The control socket lives in its own directory so containers (e.g. the
// apod-ui driver) can bind-mount the *directory* and keep reaching the socket
// across daemon restarts, which recreate the socket with a new inode.
const defaultSocketPath = "/run/apod/apod.sock"

type Server struct {
	handler *Handler
	router  *chi.Mux
}

func New(e *engine.Engine) *Server {
	h := NewHandler(e)
	r := chi.NewRouter()

	r.Use(RecoveryMiddleware)
	r.Use(LoggingMiddleware)
	r.Use(RateLimitMiddleware(60, 1*time.Minute)) // 60 requests per minute per IP

	// Unauthenticated, first-run only. Tight rate limit on both.
	r.With(RateLimitMiddleware(10, 1*time.Minute)).
		Get("/api/v1/setup/status", h.SetupStatusHandler)
	r.With(RateLimitMiddleware(5, 1*time.Minute)).
		Post("/api/v1/setup", h.SetupHandler)

	// Login is unauthenticated by design and gets a tighter rate limit to
	// slow down password brute-forcing (bcrypt also makes attempts costly).
	r.With(RateLimitMiddleware(10, 1*time.Minute)).
		Post("/api/v1/auth/login", h.AuthLoginHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(AuthMiddleware(e))
		// Enforce scoped-token (PAT) abilities for everything below. Full
		// credentials (session, API key, Unix socket) pass through untouched.
		r.Use(AbilityMiddleware)

		// Session / identity
		r.Get("/auth/me", h.AuthMeHandler)
		r.Post("/auth/logout", h.AuthLogoutHandler)

		// Two-factor (self-service; blocked for scoped tokens)
		r.Post("/auth/2fa/setup", h.TwoFactorSetupHandler)
		r.Post("/auth/2fa/enable", h.TwoFactorEnableHandler)
		r.Post("/auth/2fa/disable", h.TwoFactorDisableHandler)

		// Scoped personal access tokens (blocked for scoped tokens themselves)
		r.Post("/tokens", h.CreateAPITokenHandler)
		r.Get("/tokens", h.ListAPITokensHandler)
		r.Delete("/tokens", h.RevokeAPITokenHandler)

		// Password management (admin: anyone, user: self only)
		r.Post("/users/{name}/password", h.SetUserPasswordHandler)

		r.Post("/sites", h.CreateSite)
		r.Get("/sites", h.ListSites)
		r.Get("/sites/{domain}", h.GetSite)
		r.Get("/sites/{domain}/info", h.SiteInfo)
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
		r.Post("/sites/{domain}/backups/download", h.DownloadBackupHandler)
		r.Post("/sites/{domain}/backups/restore", h.RestoreBackupHandler)
		r.Delete("/sites/{domain}/backups", h.DeleteBackupHandler)

		// Backup schedules
		r.Post("/sites/{domain}/backups/schedule", h.AddBackupScheduleHandler)
		r.Get("/sites/{domain}/backups/schedule", h.ListBackupSchedulesHandler)
		r.Delete("/sites/{domain}/backups/schedule", h.RemoveBackupScheduleHandler)

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

		// Terminal (secure token-based container exec)
		r.Post("/sites/{domain}/terminal", h.CreateTerminalTokenHandler)

		// Clone
		r.Post("/sites/{domain}/clone", h.CloneSiteHandler)

		// Export / Import (migration)
		r.Post("/sites/{domain}/export", h.ExportSiteHandler)
		r.Post("/import", h.ImportSiteHandler)

		// Database
		r.Get("/sites/{domain}/db/export", h.DBExportHandler)
		r.Post("/sites/{domain}/db/import", h.DBImportHandler)

		// Cron jobs
		r.Post("/sites/{domain}/cron", h.AddCronJobHandler)
		r.Get("/sites/{domain}/cron", h.ListCronJobsHandler)
		r.Delete("/sites/{domain}/cron", h.RemoveCronJobHandler)

		// Proxy rules
		r.Post("/sites/{domain}/proxy", h.AddProxyRuleHandler)
		r.Get("/sites/{domain}/proxy", h.ListProxyRulesHandler)
		r.Delete("/sites/{domain}/proxy", h.RemoveProxyRuleHandler)

		// IP blocking
		r.Post("/sites/{domain}/ip/allow", h.AllowIPHandler)
		r.Post("/sites/{domain}/ip/block", h.BlockIPHandler)
		r.Post("/sites/{domain}/ip/unblock", h.UnblockIPHandler)
		r.Get("/sites/{domain}/ip", h.ListIPRulesHandler)

		// FTP
		r.Post("/sites/{domain}/ftp", h.AddFTPAccountHandler)
		r.Get("/sites/{domain}/ftp", h.ListFTPAccountsHandler)
		r.Delete("/sites/{domain}/ftp/{username}", h.RemoveFTPAccountHandler)

		// Admin-only routes
		r.Group(func(r chi.Router) {
			r.Use(AdminOnlyMiddleware)

			// Storage configs (contain cloud/SFTP credentials — admin only)
			r.Post("/storage", h.AddStorageConfigHandler)
			r.Get("/storage", h.ListStorageConfigsHandler)
			r.Delete("/storage/{name}", h.RemoveStorageConfigHandler)

			// Firewall
			r.Get("/firewall", h.FirewallStatusHandler)
			r.Get("/firewall/rules", h.FirewallRulesHandler)
			r.Post("/firewall/enable", h.FirewallEnableHandler)
			r.Post("/firewall/allow", h.FirewallAllowHandler)
			r.Post("/firewall/allow-from", h.FirewallAllowFromHandler)
			r.Post("/firewall/deny", h.FirewallDenyHandler)
			r.Post("/firewall/delete", h.FirewallDeleteHandler)

			// Custom drivers (admin-managed driver definitions)
			r.Get("/drivers/{name}", h.GetDriverHandler)
			r.Post("/drivers", h.SaveDriverHandler)
			r.Delete("/drivers/{name}", h.DeleteDriverHandler)

			// SSH Keys
			r.Post("/ssh-keys", h.AddSSHKeyHandler)
			r.Get("/ssh-keys", h.ListSSHKeysHandler)
			r.Delete("/ssh-keys/{name}", h.RemoveSSHKeyHandler)

			// System
			r.Get("/version", h.VersionHandler)
			r.Get("/update/check", h.CheckUpdateHandler)
			r.Post("/update", h.UpdateHandler)
			r.Post("/update/drivers", h.UpdateDriversHandler)

			// Server stats
			r.Get("/server-stats", h.ServerStatsHandler)
			r.Get("/disk-usage", h.DiskUsageHandler)

			// User management
			r.Post("/users", h.CreateUserHandler)
			r.Get("/users", h.ListUsersHandler)
			r.Delete("/users/{name}", h.DeleteUserHandler)
			r.Post("/users/{name}/reset-key", h.ResetAPIKeyHandler)

			// Site ownership transfer
			r.Post("/sites/{domain}/transfer", h.TransferSiteHandler)
		})
	})

	// Terminal exec — token-based, no API key needed (token IS auth)
	r.Post("/api/v1/terminal/exec", h.TerminalExecHandler)

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
	// Unix socket connections get admin access (marked by UnixSocketMiddleware)
	handler := UnixSocketMiddleware(s.router)
	return http.Serve(listener, handler)
}

func (s *Server) ListenTCP(addr string) error {
	log.Printf("apod daemon listening on %s (TCP, plaintext — put a TLS proxy in front or use ListenTCPTLS)", addr)
	return http.ListenAndServe(addr, s.router)
}

// ListenTCPTLS serves the API over HTTPS using the given certificate and key.
func (s *Server) ListenTCPTLS(addr, certFile, keyFile string) error {
	log.Printf("apod daemon listening on %s (TLS, auth required)", addr)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return srv.ListenAndServeTLS(certFile, keyFile)
}

func (s *Server) Shutdown(ctx context.Context) error {
	os.Remove(defaultSocketPath)
	return nil
}
