package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		usedBytes := (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)
		stats.DiskTotalGB = totalBytes / 1024 / 1024 / 1024
		stats.DiskUsedGB = usedBytes / 1024 / 1024 / 1024
		// Derive the percentage from raw bytes, not the GB-truncated integers —
		// otherwise the truncation loses up to ~1GB each side and reports 0% for
		// any volume under 1GB total.
		if totalBytes > 0 {
			stats.DiskPercent = float64(usedBytes) / float64(totalBytes) * 100
		}
	}

	// Memory — read from /proc/meminfo on Linux
	data, err := os.ReadFile("/proc/meminfo")
	if err == nil {
		var memTotal, memAvail uint64
		// Parse line-by-line keyed on the field prefix. (The previous fixed
		// line[:13] slice never matched "MemTotal:" — only 9 chars — so MemTotal
		// was being recovered by accident from an earlier whole-buffer scan.)
		for _, line := range splitLines(string(data)) {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
			} else if strings.HasPrefix(line, "MemAvailable:") {
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
		// User-owned sites live under /home/<owner>/sites/<domain>, not the
		// admin data dir — SiteDir knows both layouts. Its two returns are the
		// files/ and data/ children, so measure their shared parent.
		siteRoot, _ := e.SiteDir(site.Owner, site.Domain)
		size := dirSize(filepath.Dir(siteRoot))
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
