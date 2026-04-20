package engine

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
)

func (e *Engine) GetContainerLogs(ctx context.Context, domain string, lines int) (string, error) {
	containerName := fmt.Sprintf("apod-%s-app", domain)

	tail := "100"
	if lines > 0 {
		tail = fmt.Sprintf("%d", lines)
	}

	reader, err := e.docker.cli.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return "", fmt.Errorf("get container logs: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return string(data), nil
}
