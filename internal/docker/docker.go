// Package docker reads the state of stacks from the Docker Engine API. Docker is siding's
// database: containers, networks and volumes with their labels are all it knows.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/tweinmann/siding/internal/fail"
)

// Container is what siding knows about one container.
type Container struct {
	ID     string
	Name   string
	Image  string
	Labels map[string]string
	// State is one of created, running, paused, restarting, removing, exited or dead.
	State string
	// Health is "", starting, healthy or unhealthy.
	Health string
	// HealthOutput is the output of the last healthcheck.
	HealthOutput string
	ExitCode     int
	RestartCount int
	OOMKilled    bool
	Ports        []Port
	// StartedAt is when the container last started; zero when it never did.
	StartedAt time.Time
	// Cmd is the container's command, its arguments after the image's entrypoint.
	Cmd []string
}

// Port is a published port.
type Port struct {
	HostIP        string
	HostPort      int
	ContainerPort int
}

// Network is a Docker network.
type Network struct {
	Name   string
	Labels map[string]string
}

// Volume is a Docker volume.
type Volume struct {
	Name    string
	Labels  map[string]string
	Created string
	// Size in bytes, set only by VolumeSizes; -1 when Docker does not know it.
	Size int64
}

// Helper is a short-lived container that works on volumes, such as copying one into another.
type Helper struct {
	Image string
	// Binds are volume mounts, "<volume>:<path>[:ro]".
	Binds  []string
	Cmd    []string
	Labels map[string]string
}

// Client talks to the Docker daemon that docker and docker compose use.
type Client struct {
	once sync.Once
	api  *client.Client
	err  error
}

// connect resolves the daemon the docker CLI uses: DOCKER_HOST, else the current Docker
// context's endpoint, so siding and docker compose always see the same daemon.
func (c *Client) connect() (*client.Client, error) {
	c.once.Do(func() {
		opts := []client.Opt{client.FromEnv}
		if os.Getenv("DOCKER_HOST") == "" {
			if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
				if host := strings.TrimSpace(string(out)); host != "" {
					opts = append(opts, client.WithHost(host))
				}
			}
		}
		c.api, c.err = client.New(opts...)
		if c.err != nil {
			c.err = unavailable(c.err)
		}
	})
	return c.api, c.err
}

func unavailable(err error) error {
	return fail.New(fail.DockerUnavailable, "cannot reach the Docker daemon: %v", err).
		WithHint("start Docker, or check DOCKER_HOST and docker context")
}

// Ping fails with DockerUnavailable when the daemon cannot be reached.
func (c *Client) Ping(ctx context.Context) error {
	api, err := c.connect()
	if err != nil {
		return err
	}
	if _, err := api.Ping(ctx, client.PingOptions{}); err != nil {
		return unavailable(err)
	}
	return nil
}

func labelFilters(labels map[string]string) client.Filters {
	f := client.Filters{}
	for k, v := range labels {
		if v == "" {
			f.Add("label", k)
		} else {
			f.Add("label", k+"="+v)
		}
	}
	return f
}

// NCPU returns the CPUs the Docker daemon has, the VM's on Docker Desktop.
func (c *Client) NCPU(ctx context.Context) (int, error) {
	api, err := c.connect()
	if err != nil {
		return 0, err
	}
	res, err := api.Info(ctx, client.InfoOptions{})
	if err != nil {
		return 0, unavailable(err)
	}
	return res.Info.NCPU, nil
}

// CreateVolume creates a new volume with labels. It fails when the name is taken: Docker would
// silently hand back the existing volume, and siding would write into someone else's data.
func (c *Client) CreateVolume(ctx context.Context, name string, labels map[string]string) error {
	api, err := c.connect()
	if err != nil {
		return err
	}
	if _, err := api.VolumeInspect(ctx, name, client.VolumeInspectOptions{}); err == nil {
		return fail.New(fail.Internal, "the volume %s already exists", name).
			WithHint("it may belong to another machine or an older snapshot; remove it by name with docker volume rm if it is not needed")
	} else if !cerrdefs.IsNotFound(err) {
		return unavailable(err)
	}
	if _, err := api.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: labels}); err != nil {
		return fmt.Errorf("create volume %s: %w", name, err)
	}
	return nil
}

// RemoveVolume removes a volume by its exact name.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	api, err := c.connect()
	if err != nil {
		return err
	}
	if _, err := api.VolumeRemove(ctx, name, client.VolumeRemoveOptions{}); err != nil {
		return fmt.Errorf("remove volume %s: %w", name, err)
	}
	return nil
}

// VolumeSizes returns the size of every volume by name, as docker system df -v reports it.
func (c *Client) VolumeSizes(ctx context.Context) (map[string]int64, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	res, err := api.DiskUsage(ctx, client.DiskUsageOptions{Volumes: true, Verbose: true})
	if err != nil {
		return nil, unavailable(err)
	}
	sizes := map[string]int64{}
	for _, v := range res.Volumes.Items {
		sizes[v.Name] = -1
		if v.UsageData != nil {
			sizes[v.Name] = v.UsageData.Size
		}
	}
	return sizes, nil
}

// RunHelper runs a helper container to its end, pulling its image first when it is missing,
// and removes it. It returns the exit code and the last lines of its output.
func (c *Client) RunHelper(ctx context.Context, h Helper) (exit int, output []string, err error) {
	api, err := c.connect()
	if err != nil {
		return 0, nil, err
	}
	if _, err := api.ImageInspect(ctx, h.Image); cerrdefs.IsNotFound(err) {
		pull, err := api.ImagePull(ctx, h.Image, client.ImagePullOptions{})
		if err != nil {
			return 0, nil, fmt.Errorf("pull %s: %w", h.Image, err)
		}
		if err := pull.Wait(ctx); err != nil {
			return 0, nil, fmt.Errorf("pull %s: %w", h.Image, err)
		}
	} else if err != nil {
		return 0, nil, unavailable(err)
	}
	created, err := api.ContainerCreate(ctx, helperOptions(h))
	if err != nil {
		return 0, nil, fmt.Errorf("create helper container: %w", err)
	}
	id := created.ID
	defer func() {
		_, _ = api.ContainerRemove(context.WithoutCancel(ctx), id, client.ContainerRemoveOptions{Force: true})
	}()
	if _, err := api.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return 0, nil, fmt.Errorf("start helper container: %w", err)
	}
	wait := api.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case res := <-wait.Result:
		exit = int(res.StatusCode)
	case err := <-wait.Error:
		return 0, nil, fmt.Errorf("wait for helper container: %w", err)
	}
	output, err = c.Logs(ctx, id, 20)
	return exit, output, err
}

func helperOptions(h Helper) client.ContainerCreateOptions {
	return client.ContainerCreateOptions{
		Config:     &container.Config{Image: h.Image, Cmd: h.Cmd, Labels: h.Labels},
		HostConfig: &container.HostConfig{Binds: h.Binds, NetworkMode: "none"},
	}
}

// MemTotal returns the memory the Docker daemon has, the VM's on Docker Desktop.
func (c *Client) MemTotal(ctx context.Context) (int64, error) {
	api, err := c.connect()
	if err != nil {
		return 0, err
	}
	res, err := api.Info(ctx, client.InfoOptions{})
	if err != nil {
		return 0, unavailable(err)
	}
	return res.Info.MemTotal, nil
}

// Containers returns every container, running or not, that carries all the given labels; an
// empty value matches any value.
func (c *Client) Containers(ctx context.Context, labels map[string]string) ([]Container, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	list, err := api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: labelFilters(labels)})
	if err != nil {
		return nil, unavailable(err)
	}
	out := make([]Container, 0, len(list.Items))
	for _, s := range list.Items {
		ct, err := c.inspect(ctx, api, s.ID)
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				continue // removed meanwhile
			}
			return nil, err
		}
		out = append(out, ct)
	}
	return out, nil
}

// Container returns the container with this name, or nil when there is none.
func (c *Client) Container(ctx context.Context, name string) (*Container, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	ct, err := c.inspect(ctx, api, name)
	if cerrdefs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ct, nil
}

func (c *Client) inspect(ctx context.Context, api *client.Client, id string) (Container, error) {
	res, err := api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return Container{}, err
	}
	return fromInspect(res.Container), nil
}

func fromInspect(in container.InspectResponse) Container {
	ct := Container{ID: in.ID, Name: strings.TrimPrefix(in.Name, "/"), RestartCount: in.RestartCount}
	if in.Config != nil {
		ct.Image = in.Config.Image
		ct.Labels = in.Config.Labels
		ct.Cmd = in.Config.Cmd
	}
	if ct.Labels == nil {
		ct.Labels = map[string]string{}
	}
	if s := in.State; s != nil {
		ct.State = string(s.Status)
		ct.ExitCode = s.ExitCode
		ct.OOMKilled = s.OOMKilled
		if t, err := time.Parse(time.RFC3339Nano, s.StartedAt); err == nil && t.Year() > 1 {
			ct.StartedAt = t
		}
		if h := s.Health; h != nil {
			ct.Health = string(h.Status)
			if n := len(h.Log); n > 0 && h.Log[n-1] != nil {
				ct.HealthOutput = strings.TrimSpace(h.Log[n-1].Output)
			}
		}
	}
	if in.NetworkSettings != nil {
		for port, bindings := range in.NetworkSettings.Ports {
			for _, b := range bindings {
				hostPort, err := strconv.Atoi(b.HostPort)
				if err != nil {
					continue
				}
				ip := ""
				if b.HostIP.IsValid() {
					ip = b.HostIP.String()
				}
				ct.Ports = append(ct.Ports, Port{HostIP: ip, HostPort: hostPort, ContainerPort: int(port.Num())})
			}
		}
	}
	return ct
}

// Networks returns the networks that carry all the given labels.
func (c *Client) Networks(ctx context.Context, labels map[string]string) ([]Network, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	list, err := api.NetworkList(ctx, client.NetworkListOptions{Filters: labelFilters(labels)})
	if err != nil {
		return nil, unavailable(err)
	}
	out := make([]Network, 0, len(list.Items))
	for _, n := range list.Items {
		out = append(out, Network{Name: n.Name, Labels: n.Labels})
	}
	return out, nil
}

// EnsureNetwork creates the network unless it exists and reports whether it created it.
func (c *Client) EnsureNetwork(ctx context.Context, name string, labels map[string]string) (bool, error) {
	api, err := c.connect()
	if err != nil {
		return false, err
	}
	if _, err := api.NetworkInspect(ctx, name, client.NetworkInspectOptions{}); err == nil {
		return false, nil
	} else if !cerrdefs.IsNotFound(err) {
		return false, unavailable(err)
	}
	if _, err := api.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge", Labels: labels}); err != nil {
		return false, fmt.Errorf("create network %s: %w", name, err)
	}
	return true, nil
}

// NetworkGateway returns the IPv4 gateway of a network, "" when it has none.
func (c *Client) NetworkGateway(ctx context.Context, name string) (string, error) {
	api, err := c.connect()
	if err != nil {
		return "", err
	}
	res, err := api.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspect network %s: %w", name, err)
	}
	for _, cfg := range res.Network.IPAM.Config {
		if cfg.Gateway.Is4() {
			return cfg.Gateway.String(), nil
		}
	}
	return "", nil
}

// NetworkUsers returns the names of the containers attached to a network, and false when the
// network does not exist.
func (c *Client) NetworkUsers(ctx context.Context, name string) (exists bool, users []string, err error) {
	api, err := c.connect()
	if err != nil {
		return false, nil, err
	}
	res, err := api.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, unavailable(err)
	}
	for _, e := range res.Network.Containers {
		users = append(users, e.Name)
	}
	return true, users, nil
}

// RemoveNetwork removes a network by its exact name.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	api, err := c.connect()
	if err != nil {
		return err
	}
	if _, err := api.NetworkRemove(ctx, name, client.NetworkRemoveOptions{}); err != nil {
		return fmt.Errorf("remove network %s: %w", name, err)
	}
	return nil
}

// Volumes returns the volumes that carry all the given labels.
func (c *Client) Volumes(ctx context.Context, labels map[string]string) ([]Volume, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	list, err := api.VolumeList(ctx, client.VolumeListOptions{Filters: labelFilters(labels)})
	if err != nil {
		return nil, unavailable(err)
	}
	out := make([]Volume, 0, len(list.Items))
	for _, v := range list.Items {
		out = append(out, Volume{Name: v.Name, Labels: v.Labels, Created: v.CreatedAt})
	}
	return out, nil
}

// Logs returns the last tail lines a container wrote to stdout and stderr, in order.
func (c *Client) Logs(ctx context.Context, id string, tail int) ([]string, error) {
	return c.logs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail)})
}

// Stdout returns every line a container wrote to stdout that Docker still keeps, in order.
func (c *Client) Stdout(ctx context.Context, id string) ([]string, error) {
	return c.logs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, Tail: "all"})
}

func (c *Client) logs(ctx context.Context, id string, opts client.ContainerLogsOptions) ([]string, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	rc, err := api.ContainerLogs(ctx, id, opts)
	if err != nil {
		return nil, fmt.Errorf("logs of %s: %w", id, err)
	}
	defer func() { _ = rc.Close() }()
	// Both streams go into one buffer, so lines keep the order the daemon sent them in. A
	// container with a TTY sends a plain stream, not a multiplexed one.
	var buf bytes.Buffer
	raw, err := readAll(rc)
	if err != nil {
		return nil, err
	}
	if _, err := stdcopy.StdCopy(&buf, &buf, bytes.NewReader(raw)); err != nil {
		buf.Reset()
		buf.Write(raw)
	}
	var lines []string
	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	return lines, sc.Err()
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(r)
	return b.Bytes(), err
}

// Memory returns the memory a running container uses now, counted as docker stats counts it
// (usage without the inactive page cache).
func (c *Client) Memory(ctx context.Context, id string) (uint64, error) {
	api, err := c.connect()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := api.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		return 0, fmt.Errorf("stats of %s: %w", id, err)
	}
	defer func() { _ = res.Body.Close() }()
	var stats container.StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&stats); err != nil {
		return 0, fmt.Errorf("stats of %s: %w", id, err)
	}
	return MemoryUsage(stats.MemoryStats.Usage, stats.MemoryStats.Stats), nil
}

// MemoryUsage subtracts the inactive page cache from usage: inactive_file on cgroup v2,
// total_inactive_file on cgroup v1.
func MemoryUsage(usage uint64, stats map[string]uint64) uint64 {
	for _, key := range []string{"inactive_file", "total_inactive_file"} {
		if v, ok := stats[key]; ok && v < usage {
			return usage - v
		}
	}
	return usage
}
