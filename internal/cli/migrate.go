package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export [domain]",
	Short: "Export a site to a zip file for migration",
	Long:  "Creates a self-contained export zip with site files, database dumps, volume data, and config metadata. Use 'apod import' on the target server to restore.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")

		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"output_dir": output}
		resp, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/export", args[0]), body)
		if err != nil {
			return err
		}

		var result struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("Exported %s to %s (%d MB)\n", args[0], result.Path, result.Size/1024/1024)
		fmt.Println("\nTo migrate to another server:")
		fmt.Printf("  scp %s root@target-server:/tmp/\n", result.Path)
		fmt.Printf("  ssh root@target-server apod import /tmp/%s\n", filepath.Base(result.Path))
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [file.zip]",
	Short: "Import a site from an export zip file",
	Long:  "Recreates a site from a zip file created by 'apod export'. Creates the site, restores files, databases, and config.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		owner, _ := cmd.Flags().GetString("owner")

		if flagRemote != "" {
			return importRemote(args[0], domain, owner)
		}

		// Local import via API
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{
			"path":   args[0],
			"domain": domain,
			"owner":  owner,
		}
		_, err := client.Post("/api/v1/import", body)
		if err != nil {
			return err
		}

		fmt.Printf("Site imported from %s\n", args[0])
		if domain != "" {
			fmt.Printf("Domain: %s\n", domain)
		}
		return nil
	},
}

func importRemote(zipPath, domain, owner string) error {
	// For remote import, upload the file
	f, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	fmt.Printf("Uploading %s (%d MB)...\n", zipPath, info.Size()/1024/1024)

	url := flagRemote + "/api/v1/import"
	if domain != "" {
		url += "?domain=" + domain
	}
	if owner != "" {
		url += "&owner=" + owner
	}

	req, _ := http.NewRequest("POST", url, f)
	req.Header.Set("Content-Type", "application/zip")
	if flagKey != "" {
		req.Header.Set("Authorization", "Bearer "+flagKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	json.Unmarshal(body, &apiResp)
	if !apiResp.OK {
		return fmt.Errorf("%s", apiResp.Error)
	}

	fmt.Printf("Site imported successfully\n")
	return nil
}

func init() {
	exportCmd.Flags().StringP("output", "o", ".", "Output directory for export file")
	importCmd.Flags().String("domain", "", "Override domain name (default: use domain from export)")
	importCmd.Flags().String("owner", "", "Assign to user (default: admin-owned)")
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
