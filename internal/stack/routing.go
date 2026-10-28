package stack

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
)

// Route says how the proxy reaches one HTTP service.
type Route struct {
	Service string
	// Port is the container port the proxy sends requests to.
	Port int
	// Path is the service's path on the stack's host ("/" for the root), or "" when it has
	// only its service host.
	Path string
}

// validPath keeps route paths to characters that are safe inside a Traefik rule.
var validPath = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)

// Routes derives the routing table of a project: one Route per HTTP service that is not
// hidden, sorted by service. Warnings name services siding cannot route.
func Routes(p *compose.Project) (routes []Route, warnings []string, err error) {
	paths := map[string]string{} // path -> service
	for _, name := range p.ServiceNames() {
		s := p.Services[name]
		label, hasLabel := s.Labels[LabelRoute]
		label = strings.TrimSpace(label)
		if label == "false" {
			continue
		}
		path := ""
		if hasLabel && label != "" {
			if !validPath.MatchString(label) {
				return nil, nil, fail.New(fail.InvalidRoute, "service %s: %s must be \"false\" or a path such as \"/\" or \"/api\", got %q", name, LabelRoute, label)
			}
			path = label
			if path != "/" {
				path = strings.TrimRight(path, "/")
			}
		}

		port, ports, err := httpPort(name, s)
		if err != nil {
			return nil, nil, err
		}
		if s.NetworkMode != "" {
			if path != "" {
				return nil, nil, fail.New(fail.InvalidRoute, "service %s has %s %q but uses network_mode %q, so the proxy cannot reach it", name, LabelRoute, label, s.NetworkMode).
					WithHint("remove network_mode or the route label")
			}
			if port != 0 || len(ports) > 1 {
				warnings = append(warnings, fmt.Sprintf("service %s uses network_mode %q and is not routed", name, s.NetworkMode))
			}
			continue
		}
		if path != "" && port == 0 {
			if len(ports) > 1 {
				return nil, nil, fail.New(fail.AmbiguousPort, "service %s has %s %q and the container ports %s", name, LabelRoute, label, joinInts(ports)).
					WithHint("name the HTTP port with the label %s", LabelPort)
			}
			return nil, nil, fail.New(fail.InvalidRoute, "service %s has %s %q but no container port", name, LabelRoute, label).
				WithHint("add expose: [\"<port>\"] or the label %s to the service", LabelPort)
		}
		if port == 0 {
			continue
		}
		if path != "" {
			if other, taken := paths[path]; taken {
				return nil, nil, fail.New(fail.InvalidRoute, "services %s and %s both have %s %q", other, name, LabelRoute, path)
			}
			paths[path] = name
		}
		routes = append(routes, Route{Service: name, Port: port, Path: path})
	}
	return routes, warnings, nil
}

// httpPort returns the HTTP port of a service, or 0 when it has none or several (returned as
// ports, for the error message).
func httpPort(name string, s compose.Service) (port int, ports []int, err error) {
	if v, ok := s.Labels[LabelPort]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 65535 {
			return 0, nil, fail.New(fail.InvalidRoute, "service %s: %s must be a port number, got %q", name, LabelPort, v)
		}
		return n, []int{n}, nil
	}
	if len(s.Ports) == 1 {
		return s.Ports[0], s.Ports, nil
	}
	return 0, s.Ports, nil
}

func joinInts(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ", ")
}

// Find returns the route of a service.
func Find(routes []Route, service string) (Route, bool) {
	i := slices.IndexFunc(routes, func(r Route) bool { return r.Service == service })
	if i < 0 {
		return Route{}, false
	}
	return routes[i], true
}
