package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
)

const (
	Version       = "1.0.0"
	githubRepo    = "aystro/apod"
	driverRepoURL = "https://raw.githubusercontent.com/aystro/apod/main/drivers/"
)

type GithubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (e *Engine) GetVersion() string {
	return Version
}

func (e *Engine) CheckForUpdate(ctx context.Context) (string, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return "", false, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", false, fmt.Errorf("parse release: %w", err)
	}

	latest := release.TagName
	if latest != "" && latest[0] == 'v' {
		latest = latest[1:]
	}

	hasUpdate := latest != Version
	return latest, hasUpdate, nil
}

func (e *Engine) SelfUpdate(ctx context.Context) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parse release: %w", err)
	}

	// Find binary for current OS/arch
	targetName := fmt.Sprintf("apod_%s_%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == targetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download new binary
	binResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer binResp.Body.Close()

	// Get current binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Write to temp file next to the binary
	tmpPath := execPath + ".new"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(tmp, binResp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}
	tmp.Close()

	// Make executable
	os.Chmod(tmpPath, 0755)

	// Atomic replace: rename old, rename new, remove old
	oldPath := execPath + ".old"
	os.Remove(oldPath)
	if err := os.Rename(execPath, oldPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("backup old binary: %w", err)
	}
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Try to restore old binary
		os.Rename(oldPath, execPath)
		return fmt.Errorf("install new binary: %w", err)
	}
	os.Remove(oldPath)

	e.LogActivity("server", "update", fmt.Sprintf("updated to %s", release.TagName), "success")
	return nil
}

func (e *Engine) UpdateDrivers(ctx context.Context) ([]string, error) {
	drivers := []string{"static.yaml", "wordpress.yaml", "laravel.yaml"}
	var updated []string

	for _, name := range drivers {
		url := driverRepoURL + name
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			continue
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		driverPath := fmt.Sprintf("/etc/apod/drivers/%s", name)
		if err := os.WriteFile(driverPath, data, 0644); err != nil {
			continue
		}
		updated = append(updated, name)
	}

	if len(updated) > 0 {
		e.LogActivity("server", "drivers_update", fmt.Sprintf("updated: %v", updated), "success")
	}
	return updated, nil
}
