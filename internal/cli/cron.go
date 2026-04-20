package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage per-site cron jobs",
}

var cronAddCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Add a cron job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		schedule, _ := cmd.Flags().GetString("schedule")
		command, _ := cmd.Flags().GetString("command")
		service, _ := cmd.Flags().GetString("service")
		if schedule == "" || command == "" {
			return fmt.Errorf("--schedule and --command are required")
		}
		body := map[string]string{"schedule": schedule, "command": command, "service": service}
		resp, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/cron", args[0]), body)
		if err != nil {
			return err
		}
		var result map[string]int64
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("Cron job added (ID: %d)\n", result["cron_id"])
		return nil
	},
}

var cronListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List cron jobs for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/cron", args[0]))
		if err != nil {
			return err
		}
		var jobs []struct {
			ID       int64  `json:"id"`
			Schedule string `json:"schedule"`
			Command  string `json:"command"`
			Service  string `json:"service"`
			Active   bool   `json:"active"`
		}
		json.Unmarshal(resp.Data, &jobs)
		if len(jobs) == 0 {
			fmt.Println("No cron jobs found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSCHEDULE\tCOMMAND\tSERVICE")
		for _, j := range jobs {
			c := j.Command
			if len(c) > 40 {
				c = c[:40] + "..."
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", j.ID, j.Schedule, c, j.Service)
		}
		w.Flush()
		return nil
	},
}

var cronRemoveCmd = &cobra.Command{
	Use:   "remove [domain] [cron-id]",
	Short: "Remove a cron job",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var id int64
		fmt.Sscanf(args[1], "%d", &id)
		body := map[string]int64{"id": id}
		_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/cron", args[0]))
		if err != nil {
			return err
		}
		_ = body
		fmt.Printf("Cron job %d removed\n", id)
		return nil
	},
}

func init() {
	cronAddCmd.Flags().String("schedule", "", "Cron schedule (e.g. '* * * * *')")
	cronAddCmd.Flags().String("command", "", "Command to run")
	cronAddCmd.Flags().String("service", "app", "Container service to run in")
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronRemoveCmd)
	rootCmd.AddCommand(cronCmd)
}
