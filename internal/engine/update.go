package engine

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
)

var Version = "dev"

const (
	githubRepo    = "aystro-com/apod"
	driverRepoURL = "https://raw.githubusercontent.com/aystro-com/apod/master/drivers/"
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
	release, err := fetchLatestRelease()
	if err != nil {
		return "", false, err
	}

	latest := release.TagName
	if latest != "" && latest[0] == 'v' {
		latest = latest[1:]
	}

	hasUpdate := latest != Version
	return latest, hasUpdate, nil
}

// fetchLatestRelease tries stable release first, falls back to pre-releases
func fetchLatestRelease() (*GithubRelease, error) {
	// Try stable release first
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var release GithubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err == nil && release.TagName != "" {
			return &release, nil
		}
	}

	// Fall back to all releases (includes pre-releases)
	url = fmt.Sprintf("https://api.github.com/repos/%s/releases", githubRepo)
	resp2, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	defer resp2.Body.Close()

	var releases []GithubRelease
	if err := json.NewDecoder(resp2.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}

	return &releases[0], nil
}

func (e *Engine) SelfUpdate(ctx context.Context) error {
	release, err := fetchLatestRelease()
	if err != nil {
		return err
	}

	// Find tarball for current OS/arch (goreleaser format: apod_linux_amd64.tar.gz)
	targetName := fmt.Sprintf("apod_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == targetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no release found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download tarball
	binResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	defer binResp.Body.Close()

	// Extract binary from tar.gz
	gzr, err := gzip.NewReader(binResp.Body)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var binaryData []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if hdr.Name == "apod" {
			binaryData, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read binary from archive: %w", err)
			}
			break
		}
	}

	if binaryData == nil {
		return fmt.Errorf("binary 'apod' not found in archive")
	}

	// Get current binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Write to temp file next to the binary
	tmpPath := execPath + ".new"
	if err := os.WriteFile(tmpPath, binaryData, 0755); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}

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
	drivers := []string{"static.yaml", "wordpress.yaml", "laravel.yaml", "php.yaml", "node.yaml", "unifi.yaml", "odoo.yaml"}
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
