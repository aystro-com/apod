package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var serverStatsCmd = &cobra.Command{
	Use:   "server-stats",
	Short: "Show server resource usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/server-stats")
		if err != nil {
			return err
		}
		var stats struct {
			CPUCount    int     `json:"cpu_count"`
			MemTotalMB  uint64  `json:"mem_total_mb"`
			MemUsedMB   uint64  `json:"mem_used_mb"`
			MemPercent  float64 `json:"mem_percent"`
			DiskTotalGB uint64  `json:"disk_total_gb"`
			DiskUsedGB  uint64  `json:"disk_used_gb"`
			DiskPercent float64 `json:"disk_percent"`
			SiteCount   int     `json:"site_count"`
		}
		json.Unmarshal(resp.Data, &stats)
		fmt.Printf("CPUs:     %d\n", stats.CPUCount)
		fmt.Printf("Memory:   %d/%d MB (%.1f%%)\n", stats.MemUsedMB, stats.MemTotalMB, stats.MemPercent)
		fmt.Printf("Disk:     %d/%d GB (%.1f%%)\n", stats.DiskUsedGB, stats.DiskTotalGB, stats.DiskPercent)
		fmt.Printf("Sites:    %d\n", stats.SiteCount)
		return nil
	},
}

var diskUsageCmd = &cobra.Command{
	Use:   "disk-usage",
	Short: "Show disk usage per site",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/disk-usage")
		if err != nil {
			return err
		}
		var usage []struct {
			Domain string `json:"domain"`
			SizeMB int64  `json:"size_mb"`
		}
		json.Unmarshal(resp.Data, &usage)
		if len(usage) == 0 {
			fmt.Println("No sites found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tSIZE")
		for _, u := range usage {
			fmt.Fprintf(w, "%s\t%d MB\n", u.Domain, u.SizeMB)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serverStatsCmd)
	rootCmd.AddCommand(diskUsageCmd)
}
