package engine

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/docker/docker/api/types/container"
)

// statsConcurrency bounds how many docker stats calls run at once. Each one-shot
// ContainerStats blocks ~1s while the daemon samples CPU, so fanning out turns a
// serial O(sites) wait into a near-constant one without flooding the daemon.
const statsConcurrency = 16

type SiteStats struct {
	Domain        string  `json:"domain"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryMB      float64 `json:"memory_mb"`
	MemoryLimit   float64 `json:"memory_limit_mb"`
	MemoryPercent float64 `json:"memory_percent"`
}

func (e *Engine) GetSiteStats(ctx context.Context, domain string) (*SiteStats, error) {
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}

	stats := &SiteStats{
		Domain: domain,
		Status: site.Status,
	}

	// Collect stats from all containers belonging to this site.
	// Works for both normal sites (apod-<domain>-app) and compose sites (labeled containers).
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	if len(ids) == 0 {
		// Fallback: try the old-style container name
		containerName := e.primaryServiceContainer(domain)
		if exists, _ := e.docker.ContainerExists(ctx, containerName); exists {
			ids = []string{containerName}
		}
	}

	memoryLimit := parseMemoryMB(site.RAM)

	// Sample every container concurrently — each call blocks ~1s on the daemon,
	// so a multi-container site reads in ~1s instead of N×1s.
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			cpu, memMB, ok := e.containerStats(ctx, id)
			if !ok {
				return
			}
			mu.Lock()
			stats.CPUPercent += cpu
			stats.MemoryMB += memMB
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	stats.MemoryLimit = float64(memoryLimit)
	if stats.MemoryLimit > 0 {
		stats.MemoryPercent = stats.MemoryMB / stats.MemoryLimit * 100
	}

	return stats, nil
}

// containerStats reads one container's CPU% and memory (MB) via a single
// docker stats sample. ok is false when the container has gone away.
func (e *Engine) containerStats(ctx context.Context, id string) (cpuPercent, memMB float64, ok bool) {
	resp, err := e.docker.cli.ContainerStats(ctx, id, false)
	if err != nil {
		return 0, 0, false
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var ds container.StatsResponse
	if err := json.Unmarshal(data, &ds); err != nil {
		return 0, 0, false
	}

	cpuDelta := float64(ds.CPUStats.CPUUsage.TotalUsage - ds.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(ds.CPUStats.SystemUsage - ds.PreCPUStats.SystemUsage)
	if systemDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(ds.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	memMB = float64(ds.MemoryStats.Usage) / 1024 / 1024
	return cpuPercent, memMB, true
}

func (e *Engine) GetAllStats(ctx context.Context) ([]SiteStats, error) {
	sites, err := e.db.ListSites()
	if err != nil {
		return nil, err
	}

	// Sample every running site concurrently (bounded), preserving input order.
	// Serially this was O(sites)×~1s; the dashboard card spun for many seconds.
	allStats := make([]SiteStats, len(sites))
	sem := make(chan struct{}, statsConcurrency)
	var wg sync.WaitGroup
	for i, site := range sites {
		if site.Status != "running" {
			allStats[i] = SiteStats{Domain: site.Domain, Status: site.Status}
			continue
		}
		wg.Add(1)
		go func(i int, domain string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if stats, _ := e.GetSiteStats(ctx, domain); stats != nil {
				allStats[i] = *stats
			} else {
				allStats[i] = SiteStats{Domain: domain, Status: "running"}
			}
		}(i, site.Domain)
	}
	wg.Wait()
	return allStats, nil
}
