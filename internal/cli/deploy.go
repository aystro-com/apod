package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [domain]",
	Short: "Deploy a site from git",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		branch, _ := cmd.Flags().GetString("branch")
		body := map[string]string{"branch": branch}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/deploy", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s deployed successfully\n", args[0])
		return nil
	},
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback [domain]",
	Short: "Rollback to previous deployment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/rollback", args[0]), nil)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s rolled back\n", args[0])
		return nil
	},
}

var deployListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List deployments",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/deployments", args[0]))
		if err != nil {
			return err
		}
		var deps []struct {
			ID         int64     `json:"id"`
			CommitHash string    `json:"commit_hash"`
			Branch     string    `json:"branch"`
			Status     string    `json:"status"`
			CreatedAt  time.Time `json:"created_at"`
		}
		json.Unmarshal(resp.Data, &deps)
		if len(deps) == 0 {
			fmt.Println("No deployments found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCOMMIT\tBRANCH\tSTATUS\tDATE")
		for _, d := range deps {
			hash := d.CommitHash
			if len(hash) > 7 {
				hash = hash[:7]
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", d.ID, hash, d.Branch, d.Status, d.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

func init() {
	deployCmd.Flags().String("branch", "", "Branch to deploy (default: site's configured branch)")
	deployCmd.AddCommand(deployListCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(rollbackCmd)
}
