package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{Use: "proxy", Short: "Manage proxy rules"}

var proxyAddCmd = &cobra.Command{
	Use:  "add [domain]",
	Short: "Add a proxy rule",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		ruleType, _ := cmd.Flags().GetString("type")
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")

		config := map[string]string{}
		if from != "" {
			config["from"] = from
		}
		if to != "" {
			config["to"] = to
		}
		if name != "" {
			config["name"] = name
		}
		if value != "" {
			config["value"] = value
		}
		if user != "" {
			config["user"] = user
		}
		if password != "" {
			config["password"] = password
		}

		body := map[string]interface{}{"type": ruleType, "config": config}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/proxy", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Proxy rule added (%s)\n", ruleType)
		return nil
	},
}

var proxyListCmd = &cobra.Command{
	Use:  "list [domain]",
	Short: "List proxy rules",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/proxy", args[0]))
		if err != nil {
			return err
		}
		var rules []struct {
			ID       int64  `json:"id"`
			RuleType string `json:"rule_type"`
			Config   string `json:"config"`
		}
		json.Unmarshal(resp.Data, &rules)
		if len(rules) == 0 {
			fmt.Println("No proxy rules")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tCONFIG")
		for _, r := range rules {
			fmt.Fprintf(w, "%d\t%s\t%s\n", r.ID, r.RuleType, r.Config)
		}
		w.Flush()
		return nil
	},
}

var proxyRemoveCmd = &cobra.Command{
	Use:  "remove [domain] [rule-id]",
	Short: "Remove a proxy rule",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var id int64
		fmt.Sscanf(args[1], "%d", &id)
		body := map[string]int64{"id": id}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/proxy/remove", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Rule %d removed\n", id)
		return nil
	},
}

func init() {
	proxyAddCmd.Flags().String("type", "", "Rule type (redirect, header, basic-auth)")
	proxyAddCmd.Flags().String("from", "", "Redirect from path")
	proxyAddCmd.Flags().String("to", "", "Redirect to path")
	proxyAddCmd.Flags().String("name", "", "Header name")
	proxyAddCmd.Flags().String("value", "", "Header value")
	proxyAddCmd.Flags().String("user", "", "Basic auth user")
	proxyAddCmd.Flags().String("password", "", "Basic auth password")
	proxyCmd.AddCommand(proxyAddCmd, proxyListCmd, proxyRemoveCmd)
	rootCmd.AddCommand(proxyCmd)
}
