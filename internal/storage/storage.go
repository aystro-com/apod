package storage

import (
	"context"
	"fmt"
	"io"
)

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

