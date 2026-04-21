package engine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const labelPrefix = "apod."

type Docker struct {
	cli *client.Client
}

func NewDocker() (*Docker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Docker{cli: cli}, nil
}

func (d *Docker) Close() error {
	return d.cli.Close()
}

func (d *Docker) Ping(ctx context.Context) error {
	_, err := d.cli.Ping(ctx)
	return err
}

func (d *Docker) PullImage(ctx context.Context, ref string) error {
	reader, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}

type ContainerConfig struct {
	Name        string
	Image       string
	Env         []string
	Volumes     map[string]string
	Labels      map[string]string
	NetworkName string
	MemoryMB    int64
	CPUs        float64
	Command     string
	Args        []string // raw args passed directly (not through sh -c)
	Ports       map[string]string // container_port -> host_port
	User        string            // UID:GID to run container as (e.g., "5001:5001")
	PidsLimit   int64             // max processes (default 512)
}

func (d *Docker) CreateContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	var env []string
	env = append(env, cfg.Env...)

	var mounts []mount.Mount
	for host, cont := range cfg.Volumes {
		readOnly := false
		parts := strings.SplitN(cont, ":", 2)
		target := parts[0]
		if len(parts) == 2 && parts[1] == "ro" {
			readOnly = true
		}
		mountType := mount.TypeBind
		if !strings.HasPrefix(host, "/") {
			mountType = mount.TypeVolume
		}
		mounts = append(mounts, mount.Mount{
			Type:     mountType,
			Source:   host,
			Target:   target,
			ReadOnly: readOnly,
		})
	}

	pidsLimit := int64(512)
	if cfg.PidsLimit > 0 {
		pidsLimit = cfg.PidsLimit
	}
	resources := container.Resources{
		PidsLimit: &pidsLimit,
	}
	if cfg.MemoryMB > 0 {
		resources.Memory = cfg.MemoryMB * 1024 * 1024
	}
	if cfg.CPUs > 0 {
		resources.NanoCPUs = int64(cfg.CPUs * 1e9)
	}

	var cmd []string
	if len(cfg.Args) > 0 {
		cmd = cfg.Args
	} else if cfg.Command != "" {
		cmd = []string{"sh", "-c", cfg.Command}
	}

	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for containerPort, hostPort := range cfg.Ports {
		port := nat.Port(containerPort + "/tcp")
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{{HostPort: hostPort}}
	}

	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        cfg.Image,
			Env:          env,
			Labels:       cfg.Labels,
			Cmd:          cmd,
			ExposedPorts: exposedPorts,
			User:         cfg.User,
		},
		&container.HostConfig{
			Mounts:        mounts,
			Resources:     resources,
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			PortBindings:  portBindings,
			SecurityOpt:   containerSecurityOpt(cfg.Image),
			CapDrop:       []string{"ALL"},
			CapAdd:        []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID", "NET_BIND_SERVICE"},
		},
		&network.NetworkingConfig{},
		nil,
		cfg.Name,
	)
	if err != nil {
		return "", fmt.Errorf("create container %s: %w", cfg.Name, err)
	}
	return resp.ID, nil
}

func (d *Docker) StartContainer(ctx context.Context, id string) error {
	return d.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (d *Docker) StopContainer(ctx context.Context, id string) error {
	timeout := 30
	return d.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

func (d *Docker) RemoveContainer(ctx context.Context, id string) error {
	return d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

func (d *Docker) ContainerExists(ctx context.Context, name string) (bool, error) {
	_, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *Docker) ListContainersByLabel(ctx context.Context, label, value string) ([]string, error) {
	args := filters.NewArgs()
	args.Add("label", label+"="+value)

	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, c := range containers {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

func (d *Docker) EnsureNetwork(ctx context.Context, name string) error {
	_, err := d.cli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		return nil
	}

	_, err = d.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return fmt.Errorf("create network %s: %w", name, err)
	}
	return nil
}

func (d *Docker) ConnectNetwork(ctx context.Context, networkName, containerID string) error {
	return d.cli.NetworkConnect(ctx, networkName, containerID, nil)
}

func (d *Docker) RemoveNetwork(ctx context.Context, name string) error {
	return d.cli.NetworkRemove(ctx, name)
}

func (d *Docker) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	return d.ExecInContainerAs(ctx, containerID, cmd, "")
}

func (d *Docker) ExecInContainerAs(ctx context.Context, containerID string, cmd []string, user string) (string, error) {
	exec, err := d.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		User:         user,
	})
	if err != nil {
		return "", fmt.Errorf("create exec: %w", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("attach exec: %w", err)
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return "", fmt.Errorf("read exec output: %w", err)
	}

	return string(output), nil
}

// containerSecurityOpt returns security options for a container.
// Database images (mysql, postgres, mongo, mariadb) need gosu/su to switch users,
// which is incompatible with no-new-privileges.
func containerSecurityOpt(image string) []string {
	lower := strings.ToLower(image)
	for _, db := range []string{"mysql", "mariadb", "postgres", "mongo", "redis"} {
		if strings.Contains(lower, db) {
			return nil // no security opts for database containers
		}
	}
	return []string{"no-new-privileges:true"}
}
