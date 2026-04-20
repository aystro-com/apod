package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage site databases",
}

var dbExportCmd = &cobra.Command{
	Use:   "export [domain]",
	Short: "Export database dump",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/db/export", args[0]))
		if err != nil {
			return err
		}
		var result map[string]string
		json.Unmarshal(resp.Data, &result)
		fmt.Print(result["dump"])
		return nil
	},
}

var dbImportCmd = &cobra.Command{
	Use:   "import [domain] [file]",
	Short: "Import database dump",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		data, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		body := map[string]string{"dump": string(data)}
		_, err = client.Post(fmt.Sprintf("/api/v1/sites/%s/db/import", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Database imported to %s\n", args[0])
		return nil
	},
}

func init() {
	dbCmd.AddCommand(dbExportCmd)
	dbCmd.AddCommand(dbImportCmd)
	rootCmd.AddCommand(dbCmd)
}
