package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/aystro/apod/internal/models"
)

var validUsername = regexp.MustCompile(`^[a-z][a-z0-9\-]{2,31}$`)

const (
	uidRangeStart = 5000
	uidRangeEnd   = 65000
)

func (e *Engine) CreateUser(ctx context.Context, name, role string) (*models.User, string, error) {
	if !validUsername.MatchString(name) {
		return nil, "", fmt.Errorf("invalid username: must be 3-32 lowercase alphanumeric/hyphens, starting with a letter")
	}
	if role != "admin" && role != "user" {
		return nil, "", fmt.Errorf("invalid role: must be 'admin' or 'user'")
	}

	// Find next available UID
	uid, err := e.findAvailableUID()
	if err != nil {
		return nil, "", fmt.Errorf("find UID: %w", err)
	}

	// Generate API key
	rawKey, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate API key: %w", err)
	}

	// Create Linux user with no login shell
	homeDir := filepath.Join("/home", name)
	cmd := exec.CommandContext(ctx, "useradd",
		"--system",
		"--uid", strconv.Itoa(uid),
		"--home-dir", homeDir,
		"--create-home",
		"--shell", "/usr/sbin/nologin",
		name,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("create linux user: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Set home dir ownership to root (for SFTP chroot security)
	os.Chown(homeDir, 0, 0)
	os.Chmod(homeDir, 0755)

	// Create sites directory owned by the user
	sitesDir := filepath.Join(homeDir, "sites")
	os.MkdirAll(sitesDir, 0755)
	os.Chown(sitesDir, uid, uid)

	// Create .ssh directory for authorized_keys
	sshDir := filepath.Join(homeDir, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.Chown(sshDir, uid, uid)

	// Store in database
	if err := e.db.CreateUser(name, keyHash, role, uid); err != nil {
		// Rollback Linux user
		exec.CommandContext(ctx, "userdel", "--remove", name).Run()
		return nil, "", fmt.Errorf("save user: %w", err)
	}

	// Regenerate SFTP config
	e.regenerateSFTPConfig(ctx)

	user, _ := e.db.GetUserByName(name)
	e.LogActivity("server", "user_create", fmt.Sprintf("created user %s (role: %s, uid: %d)", name, role, uid), "success")

	return user, rawKey, nil
}

func (e *Engine) DeleteUser(ctx context.Context, name string) error {
	user, err := e.db.GetUserByName(name)
	if err != nil {
		return err
	}

	// Check no sites are owned by this user
	count, err := e.db.CountSitesByOwner(name)
	if err != nil {
		return fmt.Errorf("check sites: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("user %q still owns %d site(s) — destroy or reassign them first", name, count)
	}

	// Remove Linux user and home directory
	cmd := exec.CommandContext(ctx, "userdel", "--remove", name)
	cmd.Run() // Best effort — user may not exist on this system

	// Delete from database
	if err := e.db.DeleteUser(name); err != nil {
		return err
	}

	// Regenerate SFTP config
	e.regenerateSFTPConfig(ctx)

	e.LogActivity("server", "user_delete", fmt.Sprintf("deleted user %s (uid: %d)", name, user.UID), "success")
	return nil
}

func (e *Engine) ListUsers(ctx context.Context) ([]models.User, error) {
	return e.db.ListUsers()
}

func (e *Engine) ResetAPIKey(ctx context.Context, name string) (string, error) {
	rawKey, keyHash, err := generateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	if err := e.db.UpdateUserAPIKeyHash(name, keyHash); err != nil {
		return "", err
	}

	e.LogActivity("server", "user_reset_key", fmt.Sprintf("reset API key for %s", name), "success")
	return rawKey, nil
}

func (e *Engine) GetUserByAPIKeyHash(hash string) (*models.User, error) {
	return e.db.GetUserByAPIKeyHash(hash)
}

func (e *Engine) findAvailableUID() (int, error) {
	users, _ := e.db.ListUsers()
	usedUIDs := make(map[int]bool)
	for _, u := range users {
		usedUIDs[u.UID] = true
	}

	for uid := uidRangeStart; uid < uidRangeEnd; uid++ {
		if !usedUIDs[uid] {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("no available UIDs in range %d-%d", uidRangeStart, uidRangeEnd)
}

func generateAPIKey() (raw string, hash string, err error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", err
	}
	raw = "apod_" + hex.EncodeToString(keyBytes)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// siteDir returns the file paths for a site based on ownership
func (e *Engine) SiteDir(owner, domain string) (siteRoot, dataRoot string) {
	if owner == "" {
		// Legacy/admin-owned sites
		siteRoot = filepath.Join(e.dataDir, "sites", domain, "files")
		dataRoot = filepath.Join(e.dataDir, "sites", domain, "data")
	} else {
		// User-owned sites under their home directory
		siteRoot = filepath.Join("/home", owner, "sites", domain, "files")
		dataRoot = filepath.Join("/home", owner, "sites", domain, "data")
	}
	return
}

func (e *Engine) regenerateSFTPConfig(ctx context.Context) {
	users, err := e.db.ListUsers()
	if err != nil {
		return
	}

	var config strings.Builder
	config.WriteString("# Auto-generated by apod — do not edit\n\n")

	for _, u := range users {
		if u.Role == "admin" {
			continue // Admins don't get chrooted SFTP
		}
		config.WriteString(fmt.Sprintf("Match User %s\n", u.Name))
		config.WriteString(fmt.Sprintf("    ChrootDirectory /home/%s\n", u.Name))
		config.WriteString("    ForceCommand internal-sftp\n")
		config.WriteString("    AllowTcpForwarding no\n")
		config.WriteString("    X11Forwarding no\n")
		config.WriteString("    PasswordAuthentication yes\n")
		config.WriteString("\n")
	}

	configPath := "/etc/ssh/sshd_config.d/apod-users.conf"
	os.MkdirAll("/etc/ssh/sshd_config.d", 0755)
	os.WriteFile(configPath, []byte(config.String()), 0644)

	// Validate and reload sshd
	if exec.CommandContext(ctx, "sshd", "-t").Run() == nil {
		exec.CommandContext(ctx, "systemctl", "reload", "sshd").Run()
	}
}
