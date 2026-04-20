package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

type ServerStats struct {
	CPUCount    int     `json:"cpu_count"`
	MemTotalMB  uint64  `json:"mem_total_mb"`
	MemUsedMB   uint64  `json:"mem_used_mb"`
	MemPercent  float64 `json:"mem_percent"`
	DiskTotalGB uint64  `json:"disk_total_gb"`
	DiskUsedGB  uint64  `json:"disk_used_gb"`
	DiskPercent float64 `json:"disk_percent"`
	SiteCount   int     `json:"site_count"`
}

func (e *Engine) GetServerStats(ctx context.Context) (*ServerStats, error) {
	stats := &ServerStats{
		CPUCount: runtime.NumCPU(),
	}

	// Disk usage for data dir
	var stat unix.Statfs_t
	if err := unix.Statfs(e.dataDir, &stat); err == nil {
		stats.DiskTotalGB = stat.Blocks * uint64(stat.Bsize) / 1024 / 1024 / 1024
		stats.DiskUsedGB = (stat.Blocks - stat.Bfree) * uint64(stat.Bsize) / 1024 / 1024 / 1024
		if stats.DiskTotalGB > 0 {
			stats.DiskPercent = float64(stats.DiskUsedGB) / float64(stats.DiskTotalGB) * 100
		}
	}

	// Memory — read from /proc/meminfo on Linux
	data, err := os.ReadFile("/proc/meminfo")
	if err == nil {
		var memTotal, memAvail uint64
		fmt.Sscanf(string(data), "MemTotal: %d kB\nMemFree: %d", &memTotal, &memAvail)
		// Rough parse — get MemTotal and MemAvailable
		lines := string(data)
		fmt.Sscanf(lines, "MemTotal:%d", &memTotal)
		for _, line := range splitLines(lines) {
			if len(line) > 13 && line[:13] == "MemTotal:" {
				fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
			}
			if len(line) > 13 && line[:13] == "MemAvailable:" {
				fmt.Sscanf(line, "MemAvailable: %d kB", &memAvail)
			}
		}
		stats.MemTotalMB = memTotal / 1024
		stats.MemUsedMB = (memTotal - memAvail) / 1024
		if stats.MemTotalMB > 0 {
			stats.MemPercent = float64(stats.MemUsedMB) / float64(stats.MemTotalMB) * 100
		}
	}

	// Site count
	sites, _ := e.db.ListSites()
	stats.SiteCount = len(sites)

	return stats, nil
}

type SiteDiskUsage struct {
	Domain string `json:"domain"`
	SizeMB int64  `json:"size_mb"`
}

func (e *Engine) GetDiskUsage(ctx context.Context) ([]SiteDiskUsage, error) {
	sites, err := e.db.ListSites()
	if err != nil {
		return nil, err
	}

	var usage []SiteDiskUsage
	for _, site := range sites {
		siteDir := filepath.Join(e.dataDir, "sites", site.Domain)
		size := dirSize(siteDir)
		usage = append(usage, SiteDiskUsage{Domain: site.Domain, SizeMB: size / 1024 / 1024})
	}
	return usage, nil
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
