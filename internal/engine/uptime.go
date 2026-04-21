package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type UptimeChecker struct {
	engine  *Engine
	mu      sync.Mutex
	tickers map[string]*time.Ticker
	stops   map[string]chan struct{}
}

func NewUptimeChecker(e *Engine) *UptimeChecker {
	return &UptimeChecker{
		engine:  e,
		tickers: make(map[string]*time.Ticker),
		stops:   make(map[string]chan struct{}),
	}
}

func (uc *UptimeChecker) Start() {
	checks, err := uc.engine.db.ListUptimeChecks()
	if err != nil {
		log.Printf("uptime: failed to load checks: %v", err)
		return
	}
	for _, check := range checks {
		uc.startCheck(check.SiteDomain, check.URL, check.IntervalSeconds, check.AlertWebhook)
	}
}

func (uc *UptimeChecker) Stop() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	for domain, stop := range uc.stops {
		close(stop)
		delete(uc.stops, domain)
		delete(uc.tickers, domain)
	}
}

func (uc *UptimeChecker) startCheck(domain, url string, intervalSec int, alertWebhook string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if _, exists := uc.stops[domain]; exists {
		return
	}

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	stop := make(chan struct{})
	uc.tickers[domain] = ticker
	uc.stops[domain] = stop

	go func() {
		var wasUp bool = true
		for {
			select {
			case <-stop:
				ticker.Stop()
				return
			case <-ticker.C:
				isUp, statusCode, responseMs := uc.ping(url)
				uc.engine.db.LogUptimeResult(domain, statusCode, responseMs, isUp)

				if wasUp && !isUp && alertWebhook != "" {
					uc.sendAlert(alertWebhook, domain, "down", statusCode)
				} else if !wasUp && isUp && alertWebhook != "" {
					uc.sendAlert(alertWebhook, domain, "up", statusCode)
				}
				wasUp = isUp
			}
		}
	}()
}

func (uc *UptimeChecker) stopCheck(domain string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if stop, ok := uc.stops[domain]; ok {
		close(stop)
		delete(uc.stops, domain)
		delete(uc.tickers, domain)
	}
}

func (uc *UptimeChecker) ping(rawURL string) (bool, int, int) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	start := time.Now()
	resp, err := client.Get(rawURL)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0, elapsed
	}
	defer resp.Body.Close()
	isUp := resp.StatusCode >= 200 && resp.StatusCode < 400
	return isUp, resp.StatusCode, elapsed
}

func (uc *UptimeChecker) sendAlert(webhook, domain, status string, statusCode int) {
	if err := validatePublicURL(webhook); err != nil {
		log.Printf("uptime: refusing to send alert to %s: %v", webhook, err)
		return
	}
	payload := map[string]interface{}{
		"domain":      domain,
		"status":      status,
		"status_code": statusCode,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	client.Post(webhook, "application/json", bytes.NewReader(data))
}

// validatePublicURL ensures a URL is HTTP(S) and doesn't point to private/internal IPs
func validatePublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a hostname")
	}
	// Block common internal hostnames
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "metadata.google.internal" || strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("URL points to internal host")
	}
	// Resolve and check for private IPs
	ips, err := net.LookupHost(host)
	if err != nil {
		return nil // allow if DNS resolution fails (might be external)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to private/internal IP %s", ipStr)
		}
	}
	return nil
}

// Engine methods
func (e *Engine) EnableUptime(ctx context.Context, domain, rawURL string, intervalSec int, alertWebhook string) error {
	if err := validatePublicURL(rawURL); err != nil {
		return fmt.Errorf("invalid uptime URL: %w", err)
	}
	if alertWebhook != "" {
		if err := validatePublicURL(alertWebhook); err != nil {
			return fmt.Errorf("invalid webhook URL: %w", err)
		}
	}
	if err := e.db.CreateUptimeCheck(domain, rawURL, intervalSec, alertWebhook); err != nil {
		return err
	}
	if e.uptimeChecker != nil {
		e.uptimeChecker.startCheck(domain, rawURL, intervalSec, alertWebhook)
	}
	e.LogActivity(domain, "uptime_enable", fmt.Sprintf("url=%s interval=%ds", rawURL, intervalSec), "success")
	return nil
}

func (e *Engine) DisableUptime(ctx context.Context, domain string) error {
	if err := e.db.DeleteUptimeCheck(domain); err != nil {
		return err
	}
	if e.uptimeChecker != nil {
		e.uptimeChecker.stopCheck(domain)
	}
	e.LogActivity(domain, "uptime_disable", "", "success")
	return nil
}

func (e *Engine) GetUptimeStatus(ctx context.Context, domain string) (interface{}, error) {
	check, err := e.db.GetUptimeCheck(domain)
	if err != nil {
		return nil, err
	}
	stats, err := e.db.GetUptimeStats(domain, 24)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"check": check,
		"stats": stats,
	}, nil
}

func (e *Engine) GetUptimeLogs(ctx context.Context, domain string, limit int) (interface{}, error) {
	return e.db.GetUptimeLogs(domain, limit)
}
