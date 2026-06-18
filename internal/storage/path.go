package storage

import (
	"fmt"
	"path/filepath"
	"strings"
)

// safeJoin joins base and key and guarantees the result stays within base.
// It rejects absolute keys and any key that escapes the base directory via
// "..". This protects local and SFTP backends from path-traversal in the
// storage key (defense-in-depth: keys are server-generated today).
func safeJoin(base, key string) (string, error) {
	cleanKey := filepath.Clean("/" + key) // force key to be relative to root
	joined := filepath.Join(base, cleanKey)
	cleanBase := filepath.Clean(base)
	if joined != cleanBase && !strings.HasPrefix(joined, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key %q escapes base directory", key)
	}
	return joined, nil
}
