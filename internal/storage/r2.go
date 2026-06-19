package storage

import (
	"fmt"
	"regexp"
)

// r2AccountIDRe matches a Cloudflare R2 account id (hex). Validated so it can't
// reshape the endpoint host when interpolated below.
var r2AccountIDRe = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func NewR2(config map[string]string) (*S3Storage, error) {
	accountID := config["account_id"]
	if accountID == "" {
		return nil, fmt.Errorf("r2: account_id is required")
	}
	if !r2AccountIDRe.MatchString(accountID) {
		return nil, fmt.Errorf("r2: invalid account_id")
	}
	config["endpoint"] = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	config["region"] = "auto"
	return NewS3(config)
}
