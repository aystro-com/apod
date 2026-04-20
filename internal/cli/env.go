package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage site environment variables",
}

var envSetCmd = &cobra.Command{
	Use:   "set [domain] [KEY=VALUE] [KEY=VALUE]...",
	Short: "Set environment variables",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		for _, kv := range args[1:] {
			key, value, err := splitKeyValue(kv)
			if err != nil {
				return err
			}
			body := map[string]string{"key": key, "value": value}
			_, err = client.Post(fmt.Sprintf("/api/v1/sites/%s/env", domain), body)
			if err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
			fmt.Printf("Set %s=%s\n", key, value)
		}
		return nil
	},
}

var envListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List environment variables",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/env", args[0]))
		if err != nil {
			return err
		}
		var envs map[string]string
		json.Unmarshal(resp.Data, &envs)

		if len(envs) == 0 {
			fmt.Println("No environment variables set")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		keys := make([]string, 0, len(envs))
		for k := range envs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "%s\t%s\n", k, envs[k])
		}
		w.Flush()
		return nil
	},
}

var envUnsetCmd = &cobra.Command{
	Use:   "unset [domain] [KEY] [KEY]...",
	Short: "Remove environment variables",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		for _, key := range args[1:] {
			_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/env/%s", domain, key))
			if err != nil {
				return fmt.Errorf("unset %s: %w", key, err)
			}
			fmt.Printf("Removed %s\n", key)
		}
		return nil
	},
}

func splitKeyValue(s string) (string, string, error) {
	for i, c := range s {
		if c == '=' {
			return s[:i], s[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid format %q (expected KEY=VALUE)", s)
}

func init() {
	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envUnsetCmd)
	rootCmd.AddCommand(envCmd)
}
