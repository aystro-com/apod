package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Show resource usage for all sites",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/monitor")
		if err != nil {
			return err
		}
		var stats []struct {
			Domain        string  `json:"domain"`
			Status        string  `json:"status"`
			CPUPercent    float64 `json:"cpu_percent"`
			MemoryMB      float64 `json:"memory_mb"`
			MemoryLimit   float64 `json:"memory_limit_mb"`
			MemoryPercent float64 `json:"memory_percent"`
		}
		json.Unmarshal(resp.Data, &stats)
		if len(stats) == 0 {
			fmt.Println("No sites running")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tSTATUS\tCPU%\tMEM\tMEM LIMIT\tMEM%")
		for _, s := range stats {
			fmt.Fprintf(w, "%s\t%s\t%.1f%%\t%.0fMB\t%.0fMB\t%.1f%%\n",
				s.Domain, s.Status, s.CPUPercent, s.MemoryMB, s.MemoryLimit, s.MemoryPercent)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(topCmd)
}
