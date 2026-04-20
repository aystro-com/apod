package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage deploy webhooks",
}

var webhookCreateCmd = &cobra.Command{
	Use:   "create [domain]",
	Short: "Create a deploy webhook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/webhook", args[0]), nil)
		if err != nil {
			return err
		}
		var result map[string]string
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("Webhook created for %s\n", args[0])
		fmt.Printf("URL: /webhook/%s\n", result["token"])
		fmt.Printf("Token: %s\n", result["token"])
		return nil
	},
}

var webhookListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List webhooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/webhook", args[0]))
		if err != nil {
			return err
		}
		var whs []struct {
			Token  string `json:"token"`
			Active bool   `json:"active"`
		}
		json.Unmarshal(resp.Data, &whs)
		if len(whs) == 0 {
			fmt.Println("No webhooks found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TOKEN\tACTIVE")
		for _, wh := range whs {
			fmt.Fprintf(w, "%s\t%v\n", wh.Token, wh.Active)
		}
		w.Flush()
		return nil
	},
}

var webhookDeleteCmd = &cobra.Command{
	Use:   "delete [domain]",
	Short: "Delete webhook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/webhook", args[0]))
		if err != nil {
			return err
		}
		fmt.Printf("Webhook deleted for %s\n", args[0])
		return nil
	},
}

func init() {
	webhookCmd.AddCommand(webhookCreateCmd)
	webhookCmd.AddCommand(webhookListCmd)
	webhookCmd.AddCommand(webhookDeleteCmd)
	rootCmd.AddCommand(webhookCmd)
}
