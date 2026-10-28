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
	Ports       []int
	Labels      map[string]string
	Networks    []string // keys of the service's networks, sorted
	NetworkMode string
	Environment map[string]string
}

// Resource is a top-level network or volume.
type Resource struct {
	Name     string
	External bool
}

type rawProject struct {
	Name     string                 `json:"name"`
	Services map[string]rawService  `json:"services"`
	Networks map[string]rawResource `json:"networks"`
	Volumes  map[string]rawResource `json:"volumes"`
}

type rawService struct {
	Ports []struct {
		Target   int    `json:"target"`
		Protocol string `json:"protocol"`
	} `json:"ports"`
	Expose      []json.RawMessage           `json:"expose"`
	Labels      map[string]string           `json:"labels"`
	Networks    map[string]json.RawMessage  `json:"networks"`
	NetworkMode string                      `json:"network_mode"`
	Environment map[string]*json.RawMessage `json:"environment"`
}

type rawResource struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
}

// ParseConfig reads the output of docker compose config --format json.
func ParseConfig(data []byte) (*Project, error) {
	var raw rawProject
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("read compose config: %w", err)
	}
	p := &Project{Name: raw.Name, Services: map[string]Service{}, Networks: map[string]Resource{}, Volumes: map[string]Resource{}}
	for name, rs := range raw.Services {
		s := Service{Labels: rs.Labels, NetworkMode: rs.NetworkMode, Environment: map[string]string{}}
		if s.Labels == nil {
			s.Labels = map[string]string{}
		}
		ports := map[int]bool{}
		for _, port := range rs.Ports {
			if port.Protocol == "" || port.Protocol == "tcp" {
				ports[port.Target] = true
			}
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
		p.Networks[k] = Resource(r)
	}
	for k, r := range raw.Volumes {
		p.Volumes[k] = Resource(r)
	}
	return p, nil
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
