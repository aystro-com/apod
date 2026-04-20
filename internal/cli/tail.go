package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var tailCmd = &cobra.Command{
	Use:   "tail [domain]",
	Short: "Show container logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetString("lines")

		if follow {
			containerName := fmt.Sprintf("apod-%s-app", domain)
			dockerCmd := exec.Command("docker", "logs", "-f", "--tail", lines, containerName)
			dockerCmd.Stdout = os.Stdout
			dockerCmd.Stderr = os.Stderr
			return dockerCmd.Run()
		}

		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/container-logs", domain))
		if err != nil {
			return err
		}
		var result map[string]string
		json.Unmarshal(resp.Data, &result)
		fmt.Print(result["logs"])
		return nil
	},
}

func init() {
	tailCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	tailCmd.Flags().StringP("lines", "n", "100", "Number of lines to show")
	rootCmd.AddCommand(tailCmd)
}
