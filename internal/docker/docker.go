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
	Name   string
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
	}
	if ct.Labels == nil {
		ct.Labels = map[string]string{}
	}
	if s := in.State; s != nil {
		ct.State = string(s.Status)
		ct.ExitCode = s.ExitCode
		ct.OOMKilled = s.OOMKilled
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
		out = append(out, Volume{Name: v.Name, Labels: v.Labels})
	}
	return out, nil
}

// Logs returns the last tail lines a container wrote to stdout and stderr, in order.
func (c *Client) Logs(ctx context.Context, id string, tail int) ([]string, error) {
	api, err := c.connect()
	if err != nil {
		return nil, err
	}
	rc, err := api.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail)})
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
