package storage

import "fmt"

func NewR2(config map[string]string) (*S3Storage, error) {
	accountID := config["account_id"]
	if accountID == "" {
		return nil, fmt.Errorf("r2: account_id is required")
	}
	config["endpoint"] = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	config["region"] = "auto"
	return NewS3(config)
}
