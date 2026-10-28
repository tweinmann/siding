package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/stack"
)

// proxyContainerPort is the port Traefik listens on inside siding-proxy.
const proxyContainerPort = 80

// existing is the stack of the current worktree, found through its labels.
type existing struct {
	wt      git.Worktree
	id      stack.Identity
	project string
}

func (s existing) ref(port int) stackRef {
	return stackRef{Project: s.project, Repo: s.id.Repo, Branch: s.id.Branch, Worktree: s.id.Worktree, URL: address(s.id.Host(), port, "/")}
}

// findStack returns the stack of the worktree containing the current directory: the project
// whose containers carry this worktree and branch. Asking Docker rather than recomputing the
// name keeps --repo, given once to up, out of every later command.
func (a *app) findStack(ctx context.Context) (existing, error) {
	wt, err := a.opts.Git.Worktree(ctx, a.opts.Dir)
	if err != nil {
		return existing{}, err
	}
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return existing{}, err
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelWorktree: wt.Top, stack.LabelBranch: wt.Branch})
	if err != nil {
		return existing{}, err
	}
	if len(cs) == 0 {
		return existing{}, fail.New(fail.StackNotFound, "the worktree %s has no stack for the branch %s", wt.Top, wt.Branch).
			WithHint("start it with siding up")
	}
	return existing{wt: wt, id: stack.FromLabels(cs[0].Labels), project: cs[0].Labels[stack.LabelProject]}, nil
}

// proxyPort returns the host port of the running proxy.
func (a *app) proxyPort(ctx context.Context) (int, error) {
	c, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return 0, err
	}
	if c == nil || c.State != "running" {
		return 0, fail.New(fail.ProxyNotRunning, "%s is not running", a.opts.Proxy.Container).
			WithHint("start it with siding setup")
	}
	for _, p := range c.Ports {
		if p.ContainerPort == proxyContainerPort {
			return p.HostPort, nil
		}
	}
	return 0, fail.New(fail.ProxyNotRunning, "%s publishes no port", a.opts.Proxy.Container).
		WithHint("recreate it with siding setup --port <port>")
}

// shownPort is the proxy's port for addresses in ps, which also works while the proxy is down.
func (a *app) shownPort(ctx context.Context) int {
	if port, err := a.proxyPort(ctx); err == nil {
		return port
	}
	return a.opts.Proxy.DefaultPort
}

// loadProject reads the compose configuration of a worktree as project.
func (a *app) loadProject(ctx context.Context, top, file, project string) (string, *compose.Project, error) {
	path, err := compose.FindFile(a.opts.Dir, top, file)
	if err != nil {
		return "", nil, err
	}
	data, err := a.opts.Compose.Config(ctx, compose.Invocation{Dir: top, Project: project, Files: []string{path}})
	if err != nil {
		return "", nil, err
	}
	p, err := compose.ParseConfig(data)
	if err != nil {
		return "", nil, fail.New(fail.ComposeFailed, "%v", err)
	}
	return path, p, nil
}

// redactor redacts the values of the worktree's .env and of every service's environment.
func redactor(top string, p *compose.Project) (*stack.Redactor, error) {
	var values []string
	data, err := os.ReadFile(filepath.Join(top, ".env"))
	switch {
	case err == nil:
		values = stack.DotEnvValues(string(data))
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("read .env: %w", err)
	}
	if p != nil {
		for _, s := range p.Services {
			for _, v := range s.Environment {
				values = append(values, v)
			}
		}
	}
	return stack.NewRedactor(values...), nil
}

// machineID returns dev.siding.machine for this machine.
func (a *app) machineID() (string, error) {
	osID := ""
	if a.opts.OSMachineID != nil {
		osID = a.opts.OSMachineID()
	}
	return stack.MachineID(osID, a.opts.Home)
}

// address is http://<host>[:port]<path>; port 80 is left out.
func address(host string, port int, path string) string {
	if port == 80 || port == 0 {
		return "http://" + host + path
	}
	return "http://" + host + ":" + strconv.Itoa(port) + path
}

// byService groups containers by their Compose service.
func byService(cs []docker.Container) map[string][]docker.Container {
	m := map[string][]docker.Container{}
	for _, c := range cs {
		s := c.Labels[stack.LabelService]
		m[s] = append(m[s], c)
	}
	return m
}

// table aligns rows into columns two spaces apart, each line starting with indent.
func table(indent string, rows [][]string) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	for _, r := range rows {
		_, _ = io.WriteString(tw, indent+strings.Join(r, "\t")+"\n")
	}
	_ = tw.Flush()
	if b.Len() == 0 {
		return ""
	}
	// tabwriter pads empty last columns with spaces.
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

// warnf prints a warning, on stderr.
func warnf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, "warning: "+format+"\n", args...)
}
