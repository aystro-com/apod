package storage

import (
	"context"
	"fmt"
	"io"
	neturl "net/url"
	"strings"
)

// validateEndpointURL checks a custom object-storage endpoint. It requires a
// valid http(s) URL and rejects cleartext http unless explicitly allowed, so
// credentials and backup data aren't sent in the clear by default.
func validateEndpointURL(endpoint string, allowInsecure bool) error {
	u, err := neturl.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid endpoint URL %q", endpoint)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure {
			return nil
		}
		return fmt.Errorf("endpoint must use https (set insecure_endpoint=true to allow http)")
	default:
		return fmt.Errorf("endpoint scheme must be http or https")
	}
}

type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader) error
	Download(ctx context.Context, key string, writer io.Writer) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

func New(driver string, config map[string]string) (Storage, error) {
	switch driver {
	case "local":
		dir := config["path"]
		if dir == "" {
			dir = "/var/lib/apod/backups"
		}
		return NewLocal(dir), nil
	case "s3":
		return NewS3(config)
	case "r2":
		return NewR2(config)
	case "sftp":
		return NewSFTP(config)
	default:
		return nil, fmt.Errorf("unknown storage driver: %s", driver)
	}
}

