package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var uptimeCmd = &cobra.Command{
	Use:   "uptime",
	Short: "Manage uptime monitoring",
}

var uptimeEnableCmd = &cobra.Command{
	Use:   "enable [domain]",
	Short: "Enable uptime monitoring for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		url, _ := cmd.Flags().GetString("url")
		interval, _ := cmd.Flags().GetInt("interval")
		alertWebhook, _ := cmd.Flags().GetString("alert-webhook")
		if url == "" {
			url = "https://" + args[0]
		}
		if interval == 0 {
			interval = 60
		}
		body := map[string]interface{}{"url": url, "interval": interval, "alert_webhook": alertWebhook}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/uptime", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Uptime monitoring enabled for %s (every %ds)\n", args[0], interval)
		return nil
	},
}

var uptimeDisableCmd = &cobra.Command{
	Use:   "disable [domain]",
	Short: "Disable uptime monitoring",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/uptime", args[0]))
		if err != nil {
			return err
		}
		fmt.Printf("Uptime monitoring disabled for %s\n", args[0])
		return nil
	},
}

var uptimeStatusCmd = &cobra.Command{
	Use:   "status [domain]",
	Short: "Show uptime status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/uptime", args[0]))
		if err != nil {
			return err
		}
		var result struct {
			Check struct {
				URL             string `json:"url"`
				IntervalSeconds int    `json:"interval_seconds"`
			} `json:"check"`
			Stats struct {
				UptimePercent float64 `json:"uptime_percent"`
				AvgResponseMs int     `json:"avg_response_ms"`
				TotalChecks   int     `json:"total_checks"`
			} `json:"stats"`
		}
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("URL:         %s\n", result.Check.URL)
		fmt.Printf("Interval:    %ds\n", result.Check.IntervalSeconds)
		fmt.Printf("Uptime:      %.2f%%\n", result.Stats.UptimePercent)
		fmt.Printf("Avg Resp:    %dms\n", result.Stats.AvgResponseMs)
		fmt.Printf("Checks:      %d\n", result.Stats.TotalChecks)
		return nil
	},
}

var uptimeLogsCmd = &cobra.Command{
	Use:   "logs [domain]",
	Short: "Show recent uptime checks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/uptime/logs", args[0]))
		if err != nil {
			return err
		}
		var logs []struct {
			StatusCode int       `json:"status_code"`
			ResponseMs int       `json:"response_ms"`
			IsUp       bool      `json:"is_up"`
			CheckedAt  time.Time `json:"checked_at"`
		}
		json.Unmarshal(resp.Data, &logs)
		if len(logs) == 0 {
			fmt.Println("No uptime logs found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "STATUS\tCODE\tRESPONSE\tTIME")
		for _, l := range logs {
			status := "UP"
			if !l.IsUp {
				status = "DOWN"
			}
			fmt.Fprintf(w, "%s\t%d\t%dms\t%s\n", status, l.StatusCode, l.ResponseMs, l.CheckedAt.Format("01-02 15:04:05"))
		}
		w.Flush()
		return nil
	},
}

func init() {
	uptimeEnableCmd.Flags().String("url", "", "URL to check (default: https://domain)")
	uptimeEnableCmd.Flags().Int("interval", 60, "Check interval in seconds")
	uptimeEnableCmd.Flags().String("alert-webhook", "", "Webhook URL for alerts")
	uptimeCmd.AddCommand(uptimeEnableCmd)
	uptimeCmd.AddCommand(uptimeDisableCmd)
	uptimeCmd.AddCommand(uptimeStatusCmd)
	uptimeCmd.AddCommand(uptimeLogsCmd)
	rootCmd.AddCommand(uptimeCmd)
}
