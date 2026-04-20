package engine

import (
	"context"
	"fmt"
	"os/exec"
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
	// We set soft = hard (no grace period), inodes = 0 (unlimited)
	cmd := exec.CommandContext(ctx, "setquota",
		"-u", strconv.Itoa(user.UID),
		strconv.FormatInt(totalKB, 10),  // soft block limit
		strconv.FormatInt(totalKB, 10),  // hard block limit
		"0", "0", // no inode limits
		"/", // filesystem
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
