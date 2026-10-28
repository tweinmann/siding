// Package lint finds what in a compose file breaks parallel stacks or weakens siding's verdict.
// It reads the normalised configuration and never changes a file.
package lint

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
)

// Severities, worst first.
const (
	Error   = "error"
	Warning = "warning"
	Info    = "info"
)

// Rules. Part of the --json contract.
const (
	ContainerName    = "ContainerName"
	FixedNetworkName = "FixedNetworkName"
	FixedVolumeName  = "FixedVolumeName"
	InvalidRoute     = fail.InvalidRoute
	AmbiguousPort    = fail.AmbiguousPort
	HostPath         = "HostPath"
	NetworkMode      = "NetworkMode"
	NoHealthcheck    = "NoHealthcheck"
	PublishedPort    = "PublishedPort"
	NoRoot           = "NoRoot"
)

// Finding is one problem.
type Finding struct {
	Rule     string
	Severity string
	// Subject is what the finding is about: "service web", "volume data", or "stack".
	Subject string
	Message string
	Hint    string
}

// Check lints p, the configuration of project, whose worktree is the directory every relative
// bind mount lives in.
func Check(p *compose.Project, project, worktree string) []Finding {
	var out []Finding
	add := func(rule, severity, subject, hint, format string, args ...any) {
		out = append(out, Finding{Rule: rule, Severity: severity, Subject: subject, Message: fmt.Sprintf(format, args...), Hint: hint})
	}

	routes, _, err := stack.Routes(p)
	var routeErr *fail.Error
	if errors.As(err, &routeErr) {
		add(routeErr.Code, Error, "stack", routeErr.Hint, "%s", routeErr.Message)
	}
	routed := map[string]string{}
	for _, r := range routes {
		routed[r.Service] = r.Path
	}

	for _, name := range p.ServiceNames() {
		s := p.Services[name]
		subject := "service " + name
		if s.ContainerName != "" {
			add(ContainerName, Error, subject, "remove container_name; siding names containers per stack",
				"%s sets container_name %q, so a second stack of this file cannot start", name, s.ContainerName)
		}
		for _, src := range s.Binds {
			if !within(src, worktree) {
				add(HostPath, Warning, subject, "mount a path inside the worktree, or a named volume",
					"%s mounts %s from the host, and every branch's stack writes into the same directory", name, src)
			}
		}
		if s.NetworkMode != "" {
			add(NetworkMode, Warning, subject, "remove network_mode to make the service reachable through the proxy",
				"%s uses network_mode %q, so siding cannot route it", name, s.NetworkMode)
		}
		if !s.Healthcheck && routed[name] == "" {
			add(NoHealthcheck, Warning, subject, "add a healthcheck, so Ready means the service works, not only that it started",
				"%s has no healthcheck and no route path; the verdict only knows that it runs", name)
		}
		if len(s.Published) > 0 {
			add(PublishedPort, Info, subject, fmt.Sprintf("reach it through its route, or with docker compose -p %s exec %s", project, name),
				"%s publishes %s; siding resets published ports", name, joinPorts(s.Published))
		}
	}
	for _, key := range sortedKeys(p.Networks) {
		n := p.Networks[key]
		if !n.External && n.Name != project+"_"+key {
			add(FixedNetworkName, Error, "network "+key, "remove name:, or declare it external if stacks are meant to share it",
				"the network %s has the fixed name %q, which every stack of this file would share", key, n.Name)
		}
	}
	for _, key := range sortedKeys(p.Volumes) {
		v := p.Volumes[key]
		if v.External || v.Name == project+"_"+key {
			continue
		}
		if v.Labels[stack.LabelShared] != "" && v.Labels[stack.LabelShared] != "false" {
			add(FixedVolumeName, Info, "volume "+key, "",
				"the volume %s has the fixed name %q and is shared on purpose (%s)", key, v.Name, stack.LabelShared)
			continue
		}
		add(FixedVolumeName, Error, "volume "+key, fmt.Sprintf("remove name:, or label it %s: \"true\" if stacks are meant to share it", stack.LabelShared),
			"the volume %s has the fixed name %q, so every stack of this file would share its data", key, v.Name)
	}
	if len(routes) > 0 && routeErr == nil {
		root := false
		for _, r := range routes {
			root = root || r.Path == "/"
		}
		if !root {
			add(NoRoot, Info, "stack", fmt.Sprintf("give the main service %s: \"/\"", stack.LabelRoute),
				"no service is the root of the stack's address; it answers 404 at /")
		}
	}

	rank := map[string]int{Error: 0, Warning: 1, Info: 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// HasErrors reports whether a finding is an error.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func joinPorts(ps []int) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = fmt.Sprint(p)
	}
	return strings.Join(s, ", ")
}

func sortedKeys(m map[string]compose.Resource) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
