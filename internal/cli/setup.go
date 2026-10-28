package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/budget"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

func newSetupCommand(a *app) *cobra.Command {
	var asJSON bool
	var f setupFlags
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Start the shared proxy siding-proxy on 127.0.0.1",
		Long: `Start the shared proxy siding-proxy: Traefik on the network "siding", published on
127.0.0.1 only. Without --port it keeps the port of a running proxy, and uses 80 otherwise;
without --budget it keeps the budget set before. Running setup again recreates the proxy only
when something differs.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if f.port < 0 || f.port > 65535 {
				return fail.New(fail.InvalidArguments, "--port must be between 1 and 65535, got %d", f.port)
			}
			doc, err := a.setup(cmd, f)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			verb := map[string]string{"created": "started", "updated": "restarted", "unchanged": "already runs"}[doc.State]
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s, memory budget %s\n", doc.Container, verb,
				address("*.localhost", doc.Port, "/"), bytesText(uint64(doc.BudgetBytes)))
			return err
		},
	}
	cmd.Flags().IntVar(&f.port, "port", 0, "port on 127.0.0.1 for the proxy (default: the running proxy's, else 80)")
	cmd.Flags().StringVar(&f.budget, "budget", "", "memory budget for all stacks, e.g. 12g; \"default\" for 75% of Docker's memory (default: the budget set before)")
	registerJSON(cmd, &asJSON)
	return cmd
}

type setupFlags struct {
	port   int
	budget string
}

func (a *app) setup(cmd *cobra.Command, f setupFlags) (setupDocument, error) {
	ctx := cmd.Context()
	cfg := a.opts.Proxy
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return setupDocument{}, err
	}
	machine, err := a.machineID()
	if err != nil {
		return setupDocument{}, err
	}
	current, err := a.opts.Docker.Container(ctx, cfg.Container)
	if err != nil {
		return setupDocument{}, err
	}
	settings := proxy.Settings{Port: f.port, Machine: machine}
	if settings.Port == 0 {
		settings.Port = cfg.DefaultPort
		if p := hostPort(current); current != nil && current.State == "running" && p != 0 {
			settings.Port = p
		}
	}
	switch f.budget {
	case "":
		if current != nil {
			settings.Budget, _ = strconv.ParseInt(current.Labels[stack.LabelBudget], 10, 64)
		}
	case "default":
	default:
		if settings.Budget, err = budget.ParseSize(f.budget); err != nil {
			return setupDocument{}, fail.New(fail.InvalidArguments, "--budget %q: %v", f.budget, err)
		}
	}

	if _, err := a.opts.Docker.EnsureNetwork(ctx, cfg.Network, map[string]string{stack.LabelMachine: machine}); err != nil {
		return setupDocument{}, err
	}
	// Where the proxy finds siding ui (design D2): on the network's gateway when the daemon
	// runs on this machine, through Docker's host-gateway when it runs in a VM.
	settings.HostAlias = proxy.HostGateway
	gateway, err := a.opts.Docker.NetworkGateway(ctx, cfg.Network)
	if err != nil {
		return setupDocument{}, err
	}
	if gateway != "" && a.opts.IsLocalAddress != nil && a.opts.IsLocalAddress(gateway) {
		settings.HostAlias = gateway
	}
	dir := filepath.Join(a.opts.Home, ".siding", "proxy", cfg.Project)
	settings.RoutesDir = filepath.Join(dir, "routes")
	if err := os.MkdirAll(settings.RoutesDir, 0o755); err != nil {
		return setupDocument{}, fmt.Errorf("write the proxy's compose file: %w", err)
	}
	file := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(file, proxy.ComposeFile(cfg, settings), 0o644); err != nil {
		return setupDocument{}, fmt.Errorf("write the proxy's compose file: %w", err)
	}
	// Compose recreates the container only when the file differs, so it decides whether the
	// proxy is up to date; the container's ID tells what happened (design D6).
	inv := compose.Invocation{Dir: dir, Project: cfg.Project, Files: []string{file}}
	err = a.opts.Compose.Run(ctx, inv, []string{"up", "-d", "--wait", "--wait-timeout", "60", "--remove-orphans"}, cmd.ErrOrStderr(), cmd.ErrOrStderr())
	if err != nil {
		return setupDocument{}, portInUse(err, settings.Port)
	}
	after, err := a.opts.Docker.Container(ctx, cfg.Container)
	if err != nil {
		return setupDocument{}, err
	}
	doc := setupDocument{header: newHeader(kindSetup), Container: cfg.Container, Network: cfg.Network,
		Image: cfg.Image, Port: settings.Port, URL: address("*.localhost", settings.Port, "/"), State: "updated"}
	switch {
	case current == nil:
		doc.State = "created"
	case after != nil && after.ID == current.ID && current.State == "running":
		doc.State = "unchanged"
	}
	if doc.BudgetBytes, err = a.budget(ctx, after); err != nil {
		return setupDocument{}, err
	}
	return doc, nil
}

// hostPort is the port on the host a proxy container publishes, or 0.
func hostPort(c *docker.Container) int {
	if c == nil {
		return 0
	}
	for _, p := range c.Ports {
		if p.ContainerPort == proxyContainerPort {
			return p.HostPort
		}
	}
	return 0
}

// budget is the memory budget: the proxy's label, else 75 % of Docker's memory.
func (a *app) budget(ctx context.Context, proxyContainer *docker.Container) (int64, error) {
	if proxyContainer != nil {
		if b, err := strconv.ParseInt(proxyContainer.Labels[stack.LabelBudget], 10, 64); err == nil && b > 0 {
			return b, nil
		}
	}
	total, err := a.opts.Docker.MemTotal(ctx)
	if err != nil {
		return 0, err
	}
	return budget.Default(total), nil
}

// portInUse turns Compose's "address already in use" into PortInUse.
func portInUse(err error, port int) error {
	var e *fail.Error
	if errors.As(err, &e) {
		msg := strings.ToLower(e.Message)
		if strings.Contains(msg, "address already in use") || strings.Contains(msg, "port is already allocated") ||
			strings.Contains(msg, "ports are not available") {
			return fail.New(fail.PortInUse, "port %d on 127.0.0.1 is in use", port).
				WithHint("free it, or choose another port with siding setup --port 8080")
		}
	}
	return err
}
