package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ApplyDiskQuota sets Linux disk quota for a user based on their total site storage limits.
// Uses setquota to enforce a hard block limit on the user's UID.
// Requires quota tools installed and quotas enabled on the filesystem.
func (e *Engine) ApplyDiskQuota(ctx context.Context, owner string) error {
	if owner == "" {
		return nil // admin sites have no quota
	}

	user, err := e.db.GetUserByName(owner)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// Sum all storage limits for this user's sites
	sites, err := e.db.ListSitesByOwner(owner)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	var totalMB int64
	for _, s := range sites {
		totalMB += parseStorageMB(s.Storage)
	}

	if totalMB == 0 {
		return nil // no storage limits set
	}

	// Convert to KB for setquota (block size = 1KB)
	totalKB := totalMB * 1024

	// setquota -u <uid> <soft-block> <hard-block> <soft-inode> <hard-inode> <filesystem>
	// We set soft = hard (no grace period), inodes = 0 (unlimited).
	// Quotas are per-filesystem: the user's sites live under /home/<owner>, so
	// target the mount backing that path — a quota on "/" never limits anything
	// when /home is its own partition.
	cmd := exec.CommandContext(ctx, "setquota",
		"-u", strconv.Itoa(user.UID),
		strconv.FormatInt(totalKB, 10), // soft block limit
		strconv.FormatInt(totalKB, 10), // hard block limit
		"0", "0",                       // no inode limits
		mountPointFor(filepath.Join("/home", owner)),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Quota tools may not be installed — log but don't fail site creation
		e.LogActivity(owner, "quota_error", fmt.Sprintf("setquota failed: %s", strings.TrimSpace(string(output))), "warning")
		return nil
	}

	e.LogActivity(owner, "quota_set", fmt.Sprintf("disk quota set to %dMB for user %s", totalMB, owner), "success")
	return nil
}

// mountPointFor returns the mount point of the filesystem backing path, by
// picking the longest mount point in /proc/self/mounts that is a path-prefix
// of the (cleaned) target. Falls back to "/" when the table can't be read.
func mountPointFor(path string) string {
	data, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return "/"
	}
	path = filepath.Clean(path)
	best := "/"
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mp := fields[1]
		if mp == path || mp == "/" ||
			(strings.HasPrefix(path, mp) && len(mp) > 1 && path[len(mp)] == '/') {
			if len(mp) > len(best) {
				best = mp
			}
		}
	}
	return best
}

// parseStorageMB parses storage strings like "5G", "500M", "1T" into MB
func parseStorageMB(s string) int64 {
	if s == "" || s == "0" {
		return 0
	}
	s = strings.TrimSpace(strings.ToUpper(s))
	if len(s) < 2 {
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}

	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}

	switch suffix {
	case 'M':
		return num
	case 'G':
		return num * 1024
	case 'T':
		return num * 1024 * 1024
	default:
		// Might be all digits
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}
}
