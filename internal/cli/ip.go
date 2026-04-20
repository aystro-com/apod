package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var ipCmd = &cobra.Command{Use: "ip", Short: "Manage IP rules"}

var ipBlockCmd = &cobra.Command{
	Use:  "block [domain] [ip]",
	Short: "Block an IP",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"ip": args[1]}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/ip/block", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("IP %s blocked for %s\n", args[1], args[0])
		return nil
	},
}

var ipUnblockCmd = &cobra.Command{
	Use:  "unblock [domain] [ip]",
	Short: "Unblock an IP",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"ip": args[1]}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/ip/unblock", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("IP %s unblocked\n", args[1])
		return nil
	},
}

var ipListCmd = &cobra.Command{
	Use:  "list [domain]",
	Short: "List IP rules",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/ip", args[0]))
		if err != nil {
			return err
		}
		var rules []struct {
			IP     string `json:"ip"`
			Action string `json:"action"`
		}
		json.Unmarshal(resp.Data, &rules)
		if len(rules) == 0 {
			fmt.Println("No IP rules")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "IP\tACTION")
		for _, r := range rules {
			fmt.Fprintf(w, "%s\t%s\n", r.IP, r.Action)
		}
		w.Flush()
		return nil
	},
}

func init() {
	ipCmd.AddCommand(ipBlockCmd, ipUnblockCmd, ipListCmd)
	rootCmd.AddCommand(ipCmd)
}
