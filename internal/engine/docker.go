package engine

import (
	"bytes"
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
	"github.com/docker/docker/pkg/stdcopy"
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
		// Pull failed — fall back to a locally-present image if there is one
		// (air-gapped hosts, pre-loaded or locally-built images).
		if _, inspErr := d.cli.ImageInspect(ctx, ref); inspErr == nil {
			return nil
		}
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
	Args        []string          // raw args passed directly (not through sh -c)
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

	hostConfig := &container.HostConfig{
		Mounts:        mounts,
		Resources:     resources,
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		PortBindings:  portBindings,
		SecurityOpt:   containerSecurityOpt(cfg.Image),
		CapDrop:       []string{"ALL"},
		CapAdd:        []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID", "NET_BIND_SERVICE"},
	}
	netConfig := &network.NetworkingConfig{}
	// When a network is named, create the container *on that network only* so it
	// never joins Docker's shared default bridge — that bridge would otherwise
	// give every container a flat L3 path to every other site's containers by
	// raw IP. This is what keeps sites isolated from one another.
	if cfg.NetworkName != "" {
		hostConfig.NetworkMode = container.NetworkMode(cfg.NetworkName)
		netConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			cfg.NetworkName: {},
		}
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
		hostConfig,
		netConfig,
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

// ListContainersByLabels returns containers matching all of the given labels.
func (d *Docker) ListContainersByLabels(ctx context.Context, labels map[string]string) ([]string, error) {
	args := filters.NewArgs()
	for k, v := range labels {
		args.Add("label", k+"="+v)
	}
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range containers {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

// InspectReplica reconstructs a ContainerConfig from an existing container so a
// new replica can be created with an identical image, env, command, mounts, and
// resource limits. Labels are returned as-is; callers override name/replica.
func (d *Docker) InspectReplica(ctx context.Context, name string) (ContainerConfig, error) {
	info, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return ContainerConfig{}, err
	}
	cfg := ContainerConfig{
		Image:   info.Config.Image,
		Env:     append([]string{}, info.Config.Env...),
		Args:    append([]string{}, info.Config.Cmd...),
		Labels:  map[string]string{},
		Volumes: map[string]string{},
	}
	for k, v := range info.Config.Labels {
		cfg.Labels[k] = v
	}
	if info.HostConfig != nil {
		if info.HostConfig.Memory > 0 {
			cfg.MemoryMB = info.HostConfig.Memory / (1024 * 1024)
		}
		if info.HostConfig.NanoCPUs > 0 {
			cfg.CPUs = float64(info.HostConfig.NanoCPUs) / 1e9
		}
		for _, m := range info.HostConfig.Mounts {
			target := m.Target
			if m.ReadOnly {
				target += ":ro"
			}
			cfg.Volumes[m.Source] = target
		}
	}
	return cfg, nil
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

// ExecInContainer runs cmd and fails if it exits non-zero. Use ExecCombined for
// interactive cases where a non-zero exit is a normal result, not an error.
func (d *Docker) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	return d.ExecInContainerAs(ctx, containerID, cmd, "")
}

// ExecInContainerAs runs cmd as user and returns combined stdout+stderr. A
// non-zero exit code is treated as an error (with the output in the message), so
// failing setup/deploy/maintenance commands surface instead of silently passing.
func (d *Docker) ExecInContainerAs(ctx context.Context, containerID string, cmd []string, user string) (string, error) {
	stdout, stderr, code, err := d.execCapture(ctx, containerID, cmd, user)
	if err != nil {
		return "", err
	}
	out := string(stdout) + string(stderr)
	if code != 0 {
		return out, fmt.Errorf("command exited %d: %s", code, strings.TrimSpace(out))
	}
	return out, nil
}

// ExecCombined runs cmd and returns combined output plus the exit code, treating
// a non-zero exit as a normal result (no error). For interactive/console use,
// where commands legitimately exit non-zero (e.g. grep with no match).
func (d *Docker) ExecCombined(ctx context.Context, containerID string, cmd []string) (output string, exitCode int, err error) {
	stdout, stderr, code, err := d.execCapture(ctx, containerID, cmd, "")
	if err != nil {
		return "", 0, err
	}
	return string(stdout) + string(stderr), code, nil
}

// ExecCaptureStdout runs cmd and returns ONLY stdout, demultiplexed from the
// Docker exec stream. Required when capturing binary or structured output such
// as a database dump: the raw stream interleaves stderr and carries 8-byte
// frame headers that would corrupt it. Fails on a non-zero exit so a broken dump
// is never stored.
func (d *Docker) ExecCaptureStdout(ctx context.Context, containerID string, cmd []string) ([]byte, error) {
	stdout, stderr, code, err := d.execCapture(ctx, containerID, cmd, "")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return stdout, fmt.Errorf("command exited %d: %s", code, strings.TrimSpace(string(stderr)))
	}
	return stdout, nil
}

// execCapture runs cmd and returns stdout, stderr and the exit code. It
// demultiplexes Docker's exec stream (which otherwise prepends an 8-byte frame
// header to each chunk and interleaves the two streams) and inspects the exec to
// recover the exit code. The returned err is only for infrastructure failures
// (create/attach/read), never for a non-zero command exit.
func (d *Docker) execCapture(ctx context.Context, containerID string, cmd []string, user string) (stdout, stderr []byte, exitCode int, err error) {
	exec, err := d.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		User:         user,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("create exec: %w", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("attach exec: %w", err)
	}
	defer resp.Close()

	var outBuf, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader); err != nil {
		return outBuf.Bytes(), errBuf.Bytes(), 0, fmt.Errorf("read exec output: %w", err)
	}

	// The stream has drained, so the command has finished; recover its exit code.
	inspect, ierr := d.cli.ContainerExecInspect(ctx, exec.ID)
	if ierr == nil {
		exitCode = inspect.ExitCode
	}
	return outBuf.Bytes(), errBuf.Bytes(), exitCode, nil
}

// dbImageRepos are official database image repositories that need gosu/su to
// switch users, which is incompatible with no-new-privileges.
var dbImageRepos = map[string]bool{
	"mysql": true, "mariadb": true, "postgres": true, "mongo": true, "redis": true,
}

// containerSecurityOpt returns security options for a container. The exemption
// is keyed off the image's exact repository name (not a substring match) so a
// driver cannot disable no-new-privileges merely by embedding "mysql" etc. in
// an arbitrary image name like "attacker/evil-mysql".
func containerSecurityOpt(image string) []string {
	if dbImageRepos[imageRepoName(image)] {
		return nil // no security opts for official database containers
	}
	return []string{"no-new-privileges:true"}
}

// imageRepoName extracts the bare repository name from an image reference,
// stripping any registry host, namespace, tag and digest. E.g.
// "docker.io/library/postgres:16" -> "postgres", "attacker/evilmysql" -> "evilmysql".
func imageRepoName(image string) string {
	ref := strings.ToLower(strings.TrimSpace(image))
	if i := strings.IndexByte(ref, '@'); i >= 0 { // strip digest
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, '/'); i >= 0 { // strip registry/namespace
		ref = ref[i+1:]
	}
	if i := strings.IndexByte(ref, ':'); i >= 0 { // strip tag
		ref = ref[:i]
	}
	return ref
}
