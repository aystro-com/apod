package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage site configuration",
}

var configGetCmd = &cobra.Command{
	Use:   "get [domain]",
	Short: "Show site configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/config", args[0]))
		if err != nil {
			return err
		}
		var config map[string]string
		json.Unmarshal(resp.Data, &config)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		keys := make([]string, 0, len(config))
		for k := range config {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "%s\t%s\n", k, config[k])
		}
		w.Flush()
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [domain] --key [key] --value [value]",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		key, _ := cmd.Flags().GetString("set-key")
		value, _ := cmd.Flags().GetString("set-value")
		if key == "" || value == "" {
			return fmt.Errorf("--set-key and --set-value are required")
		}
		body := map[string]string{"key": key, "value": value}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/config", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Config %s=%s set for %s\n", key, value, args[0])
		return nil
	},
}

func init() {
	configSetCmd.Flags().String("set-key", "", "Config key (ram, cpu, repo, branch)")
	configSetCmd.Flags().String("set-value", "", "Config value")
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
