package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [domain]",
	Short: "Show detailed site status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		// Get site info
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s", domain))
		if err != nil {
			return err
		}
		var site struct {
			Domain    string    `json:"domain"`
			Driver    string    `json:"driver"`
			Status    string    `json:"status"`
			RAM       string    `json:"ram"`
			CPU       string    `json:"cpu"`
			Repo      string    `json:"repo"`
			Branch    string    `json:"branch"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		json.Unmarshal(resp.Data, &site)

		fmt.Printf("Domain:     %s\n", site.Domain)
		fmt.Printf("Driver:     %s\n", site.Driver)
		fmt.Printf("Status:     %s\n", site.Status)
		fmt.Printf("RAM:        %s\n", site.RAM)
		fmt.Printf("CPU:        %s\n", site.CPU)
		if site.Repo != "" {
			fmt.Printf("Repository: %s\n", site.Repo)
			fmt.Printf("Branch:     %s\n", site.Branch)
		}
		fmt.Printf("Created:    %s\n", site.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated:    %s\n", site.UpdatedAt.Format("2006-01-02 15:04:05"))

		// Get resource usage
		monResp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/monitor", domain))
		if err == nil {
			var stats struct {
				CPUPercent    float64 `json:"cpu_percent"`
				MemoryMB      float64 `json:"memory_mb"`
				MemoryLimit   float64 `json:"memory_limit_mb"`
				MemoryPercent float64 `json:"memory_percent"`
			}
			json.Unmarshal(monResp.Data, &stats)
			fmt.Println()
			fmt.Printf("CPU Usage:  %.1f%%\n", stats.CPUPercent)
			fmt.Printf("Memory:     %.0fMB / %.0fMB (%.1f%%)\n", stats.MemoryMB, stats.MemoryLimit, stats.MemoryPercent)
		}

		// Get domains
		domResp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/domains", domain))
		if err == nil {
			var domains []struct {
				Domain string `json:"domain"`
			}
			json.Unmarshal(domResp.Data, &domains)
			if len(domains) > 0 {
				fmt.Println()
				fmt.Println("Domains:")
				for _, d := range domains {
					fmt.Printf("  - %s\n", d.Domain)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
