package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage site backups",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create [domain]",
	Short: "Create a backup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		storage, _ := cmd.Flags().GetString("storage")
		body := map[string]string{"storage": storage}
		resp, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/backups", args[0]), body)
		if err != nil {
			return err
		}
		var result map[string]int64
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("Backup created (ID: %d)\n", result["backup_id"])
		return nil
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List backups for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/backups", args[0]))
		if err != nil {
			return err
		}
		var backups []struct {
			ID          int64     `json:"id"`
			StorageName string    `json:"storage_name"`
			SizeBytes   int64     `json:"size_bytes"`
			CreatedAt   time.Time `json:"created_at"`
		}
		json.Unmarshal(resp.Data, &backups)
		if len(backups) == 0 {
			fmt.Println("No backups found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTORAGE\tSIZE\tCREATED")
		for _, b := range backups {
			size := fmt.Sprintf("%d KB", b.SizeBytes/1024)
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", b.ID, b.StorageName, size, b.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore [domain] [backup-id]",
	Short: "Restore a site from backup",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var backupID int64
		fmt.Sscanf(args[1], "%d", &backupID)
		body := map[string]int64{"backup_id": backupID}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/backups/restore", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s restored from backup %d\n", args[0], backupID)
		return nil
	},
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete [domain] [backup-id]",
	Short: "Delete a backup",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var backupID int64
		fmt.Sscanf(args[1], "%d", &backupID)
		body := map[string]int64{"backup_id": backupID}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/backups/delete", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Backup %d deleted\n", backupID)
		return nil
	},
}

var backupScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage backup schedules",
}

var backupScheduleAddCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Add a backup schedule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		every, _ := cmd.Flags().GetString("every")
		storage, _ := cmd.Flags().GetString("storage")
		keep, _ := cmd.Flags().GetInt("keep")
		if every == "" {
			return fmt.Errorf("--every is required (1h, 6h, 12h, 24h, 7d, 30d)")
		}
		body := map[string]interface{}{"every": every, "storage": storage, "keep": keep}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/backups/schedule", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Backup schedule added for %s (every %s, keep %d)\n", args[0], every, keep)
		return nil
	},
}

var backupScheduleListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List backup schedules",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/backups/schedule", args[0]))
		if err != nil {
			return err
		}
		var schedules []struct {
			ID          int64  `json:"id"`
			CronExpr    string `json:"cron_expr"`
			StorageName string `json:"storage_name"`
			KeepCount   int    `json:"keep_count"`
		}
		json.Unmarshal(resp.Data, &schedules)
		if len(schedules) == 0 {
			fmt.Println("No schedules found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCRON\tSTORAGE\tKEEP")
		for _, s := range schedules {
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\n", s.ID, s.CronExpr, s.StorageName, s.KeepCount)
		}
		w.Flush()
		return nil
	},
}

var backupScheduleRemoveCmd = &cobra.Command{
	Use:   "remove [domain] [schedule-id]",
	Short: "Remove a backup schedule",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var scheduleID int64
		fmt.Sscanf(args[1], "%d", &scheduleID)
		body := map[string]int64{"schedule_id": scheduleID}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/backups/schedule/remove", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Schedule %d removed\n", scheduleID)
		return nil
	},
}

func init() {
	backupCreateCmd.Flags().String("storage", "", "Storage name (default: local)")
	backupScheduleAddCmd.Flags().String("every", "", "Backup interval (1h, 6h, 12h, 24h, 7d, 30d)")
	backupScheduleAddCmd.Flags().String("storage", "", "Storage name (default: local)")
	backupScheduleAddCmd.Flags().Int("keep", 7, "Number of backups to retain")

	backupScheduleCmd.AddCommand(backupScheduleAddCmd)
	backupScheduleCmd.AddCommand(backupScheduleListCmd)
	backupScheduleCmd.AddCommand(backupScheduleRemoveCmd)

	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	backupCmd.AddCommand(backupScheduleCmd)
	rootCmd.AddCommand(backupCmd)
}
