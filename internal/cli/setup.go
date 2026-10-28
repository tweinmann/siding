package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

func newSetupCommand(a *app) *cobra.Command {
	var asJSON bool
	var port int
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Start the shared proxy siding-proxy on 127.0.0.1",
		Long: `Start the shared proxy siding-proxy: Traefik on the network "siding", published on
127.0.0.1 only. Without --port it keeps the port of a running proxy, and uses 80 otherwise.
Running setup again changes nothing unless the port or the image differs.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if port < 0 || port > 65535 {
				return fail.New(fail.InvalidArguments, "--port must be between 1 and 65535, got %d", port)
			}
			doc, err := a.setup(cmd, port)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			verb := map[string]string{"created": "started", "updated": "restarted", "unchanged": "already runs"}[doc.State]
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\n", doc.Container, verb, address("*.localhost", doc.Port, "/"))
			return err
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port on 127.0.0.1 for the proxy (default: the running proxy's, else 80)")
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) setup(cmd *cobra.Command, port int) (setupDocument, error) {
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
	currentPort := 0
	if current != nil {
		for _, p := range current.Ports {
			if p.ContainerPort == proxyContainerPort {
				currentPort = p.HostPort
			}
		}
	}
	if port == 0 {
		port = cfg.DefaultPort
		if current != nil && current.State == "running" && currentPort != 0 {
			port = currentPort
		}
	}
	doc := setupDocument{header: newHeader(kindSetup), Container: cfg.Container, Network: cfg.Network,
		Image: cfg.Image, Port: port, URL: address("*.localhost", port, "/")}

	if current != nil && current.State == "running" && currentPort == port && current.Image == cfg.Image {
		doc.State = "unchanged"
		return doc, nil
	}
	doc.State = "created"
	if current != nil {
		doc.State = "updated"
	}

	if _, err := a.opts.Docker.EnsureNetwork(ctx, cfg.Network, map[string]string{stack.LabelMachine: machine}); err != nil {
		return setupDocument{}, err
	}
	dir := filepath.Join(a.opts.Home, ".siding", "proxy", cfg.Project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return setupDocument{}, fmt.Errorf("write the proxy's compose file: %w", err)
	}
	file := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(file, proxy.ComposeFile(cfg, port, machine), 0o644); err != nil {
		return setupDocument{}, fmt.Errorf("write the proxy's compose file: %w", err)
	}
	inv := compose.Invocation{Dir: dir, Project: cfg.Project, Files: []string{file}}
	err = a.opts.Compose.Run(ctx, inv, []string{"up", "-d", "--wait", "--wait-timeout", "60", "--remove-orphans"}, cmd.ErrOrStderr(), cmd.ErrOrStderr())
	if err != nil {
		return setupDocument{}, portInUse(err, port)
	}
	return doc, nil
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
