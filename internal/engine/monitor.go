package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
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

	containerName := fmt.Sprintf("apod-%s-app", domain)
	resp, err := e.docker.cli.ContainerStats(ctx, containerName, false)
	if err != nil {
		return stats, nil // return what we have
	}
	defer resp.Body.Close()

	var dockerStats container.StatsResponse
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &dockerStats); err != nil {
		return stats, nil
	}

	// Calculate CPU %
	cpuDelta := float64(dockerStats.CPUStats.CPUUsage.TotalUsage - dockerStats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(dockerStats.CPUStats.SystemUsage - dockerStats.PreCPUStats.SystemUsage)
	if systemDelta > 0 {
		stats.CPUPercent = (cpuDelta / systemDelta) * float64(len(dockerStats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	// Memory
	stats.MemoryMB = float64(dockerStats.MemoryStats.Usage) / 1024 / 1024
	stats.MemoryLimit = float64(dockerStats.MemoryStats.Limit) / 1024 / 1024
	if stats.MemoryLimit > 0 {
		stats.MemoryPercent = stats.MemoryMB / stats.MemoryLimit * 100
	}

	return stats, nil
}

func (e *Engine) GetAllStats(ctx context.Context) ([]SiteStats, error) {
	sites, err := e.db.ListSites()
	if err != nil {
		return nil, err
	}

	var allStats []SiteStats
	for _, site := range sites {
		if site.Status != "running" {
			allStats = append(allStats, SiteStats{Domain: site.Domain, Status: site.Status})
			continue
		}
		stats, _ := e.GetSiteStats(ctx, site.Domain)
		if stats != nil {
			allStats = append(allStats, *stats)
		}
	}
	return allStats, nil
}
