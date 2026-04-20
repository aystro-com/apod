package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage backup storage configs",
}

var storageAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a storage config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		driver, _ := cmd.Flags().GetString("driver")
		bucket, _ := cmd.Flags().GetString("bucket")
		region, _ := cmd.Flags().GetString("region")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		accessKey, _ := cmd.Flags().GetString("access-key")
		secretKey, _ := cmd.Flags().GetString("secret-key")
		accountID, _ := cmd.Flags().GetString("account-id")
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetString("port")
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")
		path, _ := cmd.Flags().GetString("path")

		if driver == "" {
			return fmt.Errorf("--driver is required (s3, r2, sftp)")
		}

		config := map[string]string{}
		if bucket != "" {
			config["bucket"] = bucket
		}
		if region != "" {
			config["region"] = region
		}
		if endpoint != "" {
			config["endpoint"] = endpoint
		}
		if accessKey != "" {
			config["access_key"] = accessKey
		}
		if secretKey != "" {
			config["secret_key"] = secretKey
		}
		if accountID != "" {
			config["account_id"] = accountID
		}
		if host != "" {
			config["host"] = host
		}
		if port != "" {
			config["port"] = port
		}
		if user != "" {
			config["user"] = user
		}
		if password != "" {
			config["password"] = password
		}
		if path != "" {
			config["path"] = path
		}

		body := map[string]interface{}{"name": args[0], "driver": driver, "config": config}
		_, err := client.Post("/api/v1/storage", body)
		if err != nil {
			return err
		}
		fmt.Printf("Storage %s added (%s)\n", args[0], driver)
		return nil
	},
}

var storageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List storage configs",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/storage")
		if err != nil {
			return err
		}
		var configs []struct {
			Name   string `json:"name"`
			Driver string `json:"driver"`
		}
		json.Unmarshal(resp.Data, &configs)
		if len(configs) == 0 {
			fmt.Println("No storage configs found")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tDRIVER")
		for _, c := range configs {
			fmt.Fprintf(w, "%s\t%s\n", c.Name, c.Driver)
		}
		w.Flush()
		return nil
	},
}

var storageRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a storage config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/storage/%s", args[0]))
		if err != nil {
			return err
		}
		fmt.Printf("Storage %s removed\n", args[0])
		return nil
	},
}

func init() {
	storageAddCmd.Flags().String("driver", "", "Storage driver (s3, r2, sftp)")
	storageAddCmd.Flags().String("bucket", "", "Bucket name")
	storageAddCmd.Flags().String("region", "", "AWS region")
	storageAddCmd.Flags().String("endpoint", "", "Custom S3 endpoint")
	storageAddCmd.Flags().String("access-key", "", "Access key")
	storageAddCmd.Flags().String("secret-key", "", "Secret key")
	storageAddCmd.Flags().String("account-id", "", "Cloudflare account ID (for R2)")
	storageAddCmd.Flags().String("host", "", "SFTP host")
	storageAddCmd.Flags().String("port", "", "SFTP port")
	storageAddCmd.Flags().String("user", "", "SFTP user")
	storageAddCmd.Flags().String("password", "", "SFTP password")
	storageAddCmd.Flags().String("path", "", "SFTP base path")

	storageCmd.AddCommand(storageAddCmd)
	storageCmd.AddCommand(storageListCmd)
	storageCmd.AddCommand(storageRemoveCmd)
	rootCmd.AddCommand(storageCmd)
}
