// Package overview holds what siding ui needs besides the page: the route file through which
// siding-proxy reaches it, and the addresses it listens on.
package overview

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"

	"github.com/tweinmann/siding/internal/proxy"
)

// Host is the overview's address on the proxy.
const Host = "siding.localhost"

// RouteFileName is the route file's name in the proxy's routes directory.
const RouteFileName = "ui.yml"

// Route is the Traefik dynamic configuration that sends Host to siding ui on port.
func Route(port int) []byte {
	return fmt.Appendf(nil, `# Written by siding ui while it runs; removed when it stops.
http:
  routers:
    siding-ui:
      rule: "Host(`+"`%s`"+`)"
      entrypoints: [web]
      service: siding-ui
  services:
    siding-ui:
      loadBalancer:
        servers:
          - url: "http://host.docker.internal:%d"
`, Host, port)
}

// WriteRoute writes the route file into dir, replacing one a killed siding ui left behind, and
// returns the function that removes it. The file appears at once (write and rename), so the
// proxy never reads half of it.
func WriteRoute(dir string, port int) (remove func() error, err error) {
	path := filepath.Join(dir, RouteFileName)
	tmp, err := os.CreateTemp(dir, ".ui-*.yml")
	if err != nil {
		return nil, fmt.Errorf("write the overview's route: %w", err)
	}
	if _, err := tmp.Write(Route(port)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("write the overview's route: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("write the overview's route: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("write the overview's route: %w", err)
	}
	return func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}, nil
}

// IsLocalAddress reports whether ip is assigned to an interface of this machine.
func IsLocalAddress(ip string) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.String() == ip {
			return true
		}
	}
	return false
}

// ListenAddrs are the addresses siding ui listens on: the loopback interface, and the proxy
// network's gateway when the proxy reaches this machine there. Never all interfaces.
func ListenAddrs(hostAlias string) []string {
	addrs := []string{"127.0.0.1"}
	if hostAlias != "" && hostAlias != proxy.HostGateway && hostAlias != "127.0.0.1" {
		addrs = append(addrs, hostAlias)
	}
	return addrs
}
