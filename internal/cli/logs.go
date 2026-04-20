package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [domain]",
	Short: "Show activity logs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var resp *apiResponse
		var err error

		if len(args) == 1 {
			resp, err = client.Get(fmt.Sprintf("/api/v1/sites/%s/logs", args[0]))
		} else {
			resp, err = client.Get("/api/v1/logs")
		}
		if err != nil {
			return err
		}

		var logs []struct {
			SiteDomain string    `json:"site_domain"`
			Action     string    `json:"action"`
			Details    string    `json:"details"`
			Result     string    `json:"result"`
			CreatedAt  time.Time `json:"created_at"`
		}
		json.Unmarshal(resp.Data, &logs)

		if len(logs) == 0 {
			fmt.Println("No activity logs found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SITE\tACTION\tDETAILS\tRESULT\tDATE")
		for _, l := range logs {
			details := l.Details
			if len(details) > 30 {
				details = details[:30] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", l.SiteDomain, l.Action, details, l.Result, l.CreatedAt.Format("01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
