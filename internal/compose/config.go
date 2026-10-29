// Package compose runs docker compose and reads the normalised configuration it prints. siding
// never parses a compose file itself: interpolation, profiles, include and extends are
// Compose's business.
package compose

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/tweinmann/siding/internal/fail"
)

// FileNames are the compose files Compose itself looks for, in its order.
var FileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// FindFile returns the compose file of a worktree: file when given (relative to dir), else the
// first of FileNames in top.
func FindFile(dir, top, file string) (string, error) {
	if file != "" {
		if !filepath.IsAbs(file) {
			file = filepath.Join(dir, file)
		}
		if _, err := os.Stat(file); err != nil {
			return "", fail.New(fail.ComposeFileNotFound, "compose file %s not found", file)
		}
		return file, nil
	}
	for _, name := range FileNames {
		path := filepath.Join(top, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fail.New(fail.ComposeFileNotFound, "no compose file in %s", top).
		WithHint("siding looks for %s; name another one with --file", strings.Join(FileNames, ", "))
}

// Project is the part of a normalised compose configuration siding needs.
type Project struct {
	Name     string
	Services map[string]Service
	Networks map[string]Resource
	Volumes  map[string]Resource
}

// ServiceNames returns the service names in the order Compose prints them (sorted).
func (p *Project) ServiceNames() []string {
	names := make([]string, 0, len(p.Services))
	for n := range p.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Service is one service of a Project.
type Service struct {
	// Ports are the distinct container ports from ports (targets) and expose, ascending.
	Ports []int
	// Published are the ports the compose file publishes on the host.
	Published   []int
	Labels      map[string]string
	Networks    []string // keys of the service's networks, sorted
	NetworkMode string
	Environment map[string]string

	ContainerName string
	// Binds are the absolute host paths of the service's bind mounts.
	Binds []string
	// Volumes are the keys of the named volumes the service mounts.
	Volumes []string
	// Healthcheck is set when the compose file gives the service a healthcheck. One from the
	// image is not visible here.
	Healthcheck bool
	// MemoryLimit is deploy.resources.limits.memory, else mem_limit, in bytes; 0 for none.
	MemoryLimit int64
	// Replicas is deploy.replicas, else scale, else 1.
	Replicas int
	// Build is set when the service is built from a Dockerfile.
	Build bool
}

// Resource is a top-level network or volume.
type Resource struct {
	Name     string
	External bool
	Labels   map[string]string
}

type rawProject struct {
	Name     string                 `json:"name"`
	Services map[string]rawService  `json:"services"`
	Networks map[string]rawResource `json:"networks"`
	Volumes  map[string]rawResource `json:"volumes"`
}

type rawService struct {
	Ports []struct {
		Target    int    `json:"target"`
		Published string `json:"published"`
		Protocol  string `json:"protocol"`
	} `json:"ports"`
	ContainerName string          `json:"container_name"`
	Build         json.RawMessage `json:"build"`
	Volumes       []struct {
		Type   string `json:"type"`
		Source string `json:"source"`
	} `json:"volumes"`
	Healthcheck *struct {
		Test    []string `json:"test"`
		Disable bool     `json:"disable"`
	} `json:"healthcheck"`
	MemLimit string `json:"mem_limit"`
	Scale    *int   `json:"scale"`
	Deploy   *struct {
		Replicas  *int `json:"replicas"`
		Resources struct {
			Limits struct {
				Memory string `json:"memory"`
			} `json:"limits"`
		} `json:"resources"`
	} `json:"deploy"`
	Expose      []json.RawMessage           `json:"expose"`
	Labels      map[string]string           `json:"labels"`
	Networks    map[string]json.RawMessage  `json:"networks"`
	NetworkMode string                      `json:"network_mode"`
	Environment map[string]*json.RawMessage `json:"environment"`
}

type rawResource struct {
	Name     string            `json:"name"`
	External bool              `json:"external"`
	Labels   map[string]string `json:"labels"`
}

// ParseConfig reads the output of docker compose config --format json.
func ParseConfig(data []byte) (*Project, error) {
	var raw rawProject
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("read compose config: %w", err)
	}
	p := &Project{Name: raw.Name, Services: map[string]Service{}, Networks: map[string]Resource{}, Volumes: map[string]Resource{}}
	for name, rs := range raw.Services {
		s := Service{Labels: rs.Labels, NetworkMode: rs.NetworkMode, Environment: map[string]string{},
			ContainerName: rs.ContainerName, Replicas: 1, Build: len(rs.Build) > 0 && string(rs.Build) != "null"}
		if s.Labels == nil {
			s.Labels = map[string]string{}
		}
		ports := map[int]bool{}
		for _, port := range rs.Ports {
			if port.Protocol == "" || port.Protocol == "tcp" {
				ports[port.Target] = true
			}
			if n, err := strconv.Atoi(port.Published); err == nil {
				s.Published = append(s.Published, n)
			}
		}
		for _, v := range rs.Volumes {
			switch v.Type {
			case "bind":
				s.Binds = append(s.Binds, v.Source)
			case "volume":
				if v.Source != "" {
					s.Volumes = append(s.Volumes, v.Source)
				}
			}
		}
		if h := rs.Healthcheck; h != nil && !h.Disable && len(h.Test) > 0 && h.Test[0] != "NONE" {
			s.Healthcheck = true
		}
		limit := rs.MemLimit
		if rs.Deploy != nil && rs.Deploy.Resources.Limits.Memory != "" {
			limit = rs.Deploy.Resources.Limits.Memory
		}
		if limit != "" {
			n, err := strconv.ParseInt(limit, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("service %s: memory limit %q", name, limit)
			}
			s.MemoryLimit = n
		}
		switch {
		case rs.Deploy != nil && rs.Deploy.Replicas != nil:
			s.Replicas = *rs.Deploy.Replicas
		case rs.Scale != nil:
			s.Replicas = *rs.Scale
		}
		for _, e := range rs.Expose {
			expanded, err := parseExpose(e)
			if err != nil {
				return nil, fmt.Errorf("service %s: %w", name, err)
			}
			for _, n := range expanded {
				ports[n] = true
			}
		}
		for n := range ports {
			s.Ports = append(s.Ports, n)
		}
		slices.Sort(s.Ports)
		for n := range rs.Networks {
			s.Networks = append(s.Networks, n)
		}
		slices.Sort(s.Networks)
		for k, v := range rs.Environment {
			if v == nil {
				continue
			}
			var str string
			if err := json.Unmarshal(*v, &str); err == nil {
				s.Environment[k] = str
			}
		}
		p.Services[name] = s
	}
	for k, r := range raw.Networks {
		p.Networks[k] = resource(r)
	}
	for k, r := range raw.Volumes {
		p.Volumes[k] = resource(r)
	}
	return p, nil
}

func resource(r rawResource) Resource {
	labels := r.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return Resource{Name: r.Name, External: r.External, Labels: labels}
}

// parseExpose expands one expose entry ("3000", "3000/tcp", "9000-9001", or a number) into
// TCP ports.
func parseExpose(raw json.RawMessage) ([]int, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, fmt.Errorf("expose entry %s", raw)
		}
		return []int{n}, nil
	}
	spec, proto, _ := strings.Cut(s, "/")
	if proto != "" && proto != "tcp" {
		return nil, nil
	}
	lo, hi, isRange := strings.Cut(spec, "-")
	first, err := strconv.Atoi(lo)
	if err != nil {
		return nil, fmt.Errorf("expose entry %q", s)
	}
	last := first
	if isRange {
		if last, err = strconv.Atoi(hi); err != nil || last < first {
			return nil, fmt.Errorf("expose entry %q", s)
		}
	}
	// Only the number of ports matters (one makes an HTTP service), so a range is its ends.
	if last > first {
		return []int{first, last}, nil
	}
	return []int{first}, nil
}
