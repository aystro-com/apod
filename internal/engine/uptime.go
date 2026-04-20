package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

func (uc *UptimeChecker) ping(url string) (bool, int, int) {
	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := client.Get(url)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0, elapsed
	}
	defer resp.Body.Close()
	isUp := resp.StatusCode >= 200 && resp.StatusCode < 400
	return isUp, resp.StatusCode, elapsed
}

func (uc *UptimeChecker) sendAlert(webhook, domain, status string, statusCode int) {
	payload := map[string]interface{}{
		"domain":      domain,
		"status":      status,
		"status_code": statusCode,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewReader(data))
}

// Engine methods
func (e *Engine) EnableUptime(ctx context.Context, domain, url string, intervalSec int, alertWebhook string) error {
	if err := e.db.CreateUptimeCheck(domain, url, intervalSec, alertWebhook); err != nil {
		return err
	}
	if e.uptimeChecker != nil {
		e.uptimeChecker.startCheck(domain, url, intervalSec, alertWebhook)
	}
	e.LogActivity(domain, "uptime_enable", fmt.Sprintf("url=%s interval=%ds", url, intervalSec), "success")
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
