package engine

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/aystro/apod/internal/models"
	"github.com/docker/docker/api/types/container"
)

// statsConcurrency bounds how many docker stats calls run at once. One-shot
// stats return immediately, but fanning out still avoids serializing the
// per-call round-trip across many containers without flooding the daemon.
const statsConcurrency = 16

// cpuSampleTTL evicts cached CPU samples for containers we haven't seen in a
// while (destroyed sites), so the cache can't grow without bound.
const cpuSampleTTL = 10 * time.Minute

// cpuSample is the previous CPU reading for a container, used to compute usage
// as a delta — so each stats call can be an instant one-shot read instead of
// waiting ~1s for the daemon to sample CPU twice itself.
type cpuSample struct {
	total  uint64
	system uint64
	onCPU  float64
	seen   time.Time
}

var (
	cpuSamples   = map[string]cpuSample{}
	cpuSamplesMu sync.Mutex
)

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

	// Sample every container concurrently. Each read is an instant one-shot.
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

// containerStats reads one container's CPU% and memory (MB) via an instant
// one-shot stats call. CPU% is computed from the delta against the previous
// cached sample, so there's no ~1s wait for the daemon to sample CPU itself.
// The first reading for a cold container reports 0% CPU; it's accurate from the
// next poll on. ok is false when the container has gone away.
func (e *Engine) containerStats(ctx context.Context, id string) (cpuPercent, memMB float64, ok bool) {
	resp, err := e.docker.cli.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return 0, 0, false
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var ds container.StatsResponse
	if err := json.Unmarshal(data, &ds); err != nil {
		return 0, 0, false
	}

	cur := cpuSample{
		total:  ds.CPUStats.CPUUsage.TotalUsage,
		system: ds.CPUStats.SystemUsage,
		onCPU:  float64(len(ds.CPUStats.CPUUsage.PercpuUsage)),
		seen:   time.Now(),
	}
	// cgroup v2 doesn't populate PercpuUsage — fall back to the reported count.
	if cur.onCPU == 0 {
		cur.onCPU = float64(ds.CPUStats.OnlineCPUs)
	}

	cpuSamplesMu.Lock()
	prev, had := cpuSamples[id]
	cpuSamples[id] = cur
	cpuSamplesMu.Unlock()

	if had {
		cpuPercent = cpuPercentFromDelta(prev, cur)
	}
	memMB = float64(ds.MemoryStats.Usage) / 1024 / 1024
	return cpuPercent, memMB, true
}

// cpuPercentFromDelta computes container CPU% from two cumulative samples, the
// same formula the docker CLI uses: container time over host time, scaled by the
// online CPU count. Returns 0 if the samples don't form a forward-moving window.
func cpuPercentFromDelta(prev, cur cpuSample) float64 {
	// Guard against counters moving backward (container/daemon restart) before
	// subtracting — uint64 underflow would otherwise yield a huge bogus value.
	if cur.total <= prev.total || cur.system <= prev.system {
		return 0
	}
	cpuDelta := float64(cur.total - prev.total)
	systemDelta := float64(cur.system - prev.system)
	if systemDelta > 0 {
		return (cpuDelta / systemDelta) * cur.onCPU * 100.0
	}
	return 0
}

// pruneCPUSamples drops cached samples for containers not seen within the TTL,
// bounding the cache to currently-active containers.
func pruneCPUSamples() {
	cutoff := time.Now().Add(-cpuSampleTTL)
	cpuSamplesMu.Lock()
	for id, s := range cpuSamples {
		if s.seen.Before(cutoff) {
			delete(cpuSamples, id)
		}
	}
	cpuSamplesMu.Unlock()
}

// GetAllStats returns live stats for sites. A non-empty owner restricts the
// result to that owner's sites; an empty owner (admin) returns all.
func (e *Engine) GetAllStats(ctx context.Context, owner string) ([]SiteStats, error) {
	var sites []models.Site
	var err error
	if owner == "" {
		sites, err = e.db.ListSites()
	} else {
		sites, err = e.db.ListSitesByOwner(owner)
	}
	if err != nil {
		return nil, err
	}

	pruneCPUSamples()

	// Sample every running site concurrently (bounded), preserving input order.
	// One-shot reads are instant, so the whole card refreshes in well under a
	// second regardless of how many sites there are.
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
