// Package proxy describes siding-proxy: one Traefik with the Docker provider, run as its own
// Compose project, through which every stack is reached by host and path.
package proxy

import (
	"fmt"
	"strings"
)

// Image is the Traefik image siding-proxy runs; the devcontainer's SIDING_TRAEFIK_IMAGE is the
// same.
const Image = "traefik:v3.7.16"

// Config names the proxy and its network. Tests use their own, so they never touch a
// developer's siding-proxy.
type Config struct {
	Project     string // Compose project
	Container   string // container name
	Network     string // Docker network every routed service joins
	Image       string
	DefaultPort int // host port on 127.0.0.1 when setup gets no --port
}

// Default is the proxy of a real installation.
func Default() Config {
	return Config{Project: "siding", Container: "siding-proxy", Network: "siding", Image: Image, DefaultPort: 80}
}

// RoutesMount is where the proxy reads route files that siding ui writes.
const RoutesMount = "/etc/siding/routes"

// HostGateway is the host alias when the Docker daemon runs in a VM: Docker's own name for the
// machine outside it.
const HostGateway = "host-gateway"

// Settings are what one setup chooses for the proxy.
type Settings struct {
	Port    int
	Machine string
	// Budget is the memory budget in bytes, 0 for the default.
	Budget int64
	// Builds is the number of build slots, 0 for the default.
	Builds int
	// RoutesDir is the directory on this machine with the route files of siding ui.
	RoutesDir string
	// HostAlias is what host.docker.internal resolves to in the proxy: the proxy network's
	// gateway when that is an address of this machine, else HostGateway (design D2).
	HostAlias string
}

// ComposeFile renders the proxy's compose file. The constraint keeps Traefik to containers
// siding labelled for this proxy's network, so foreign Traefik labels on the same daemon are
// never exposed.
func ComposeFile(c Config, s Settings) []byte {
	constraint := fmt.Sprintf("LabelRegex(`dev.siding.repo`, `.+`) && Label(`traefik.docker.network`, `%s`)", c.Network)
	labels := fmt.Sprintf("      dev.siding.machine: %q\n      dev.siding.host-alias: %q\n", s.Machine, s.HostAlias)
	if s.Budget > 0 {
		labels += fmt.Sprintf("      dev.siding.budget: \"%d\"\n", s.Budget)
	}
	if s.Builds > 0 {
		labels += fmt.Sprintf("      dev.siding.builds: \"%d\"\n", s.Builds)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `# Written by siding setup. Do not edit: siding setup rewrites it.
services:
  traefik:
    image: %q
    container_name: %q
    restart: unless-stopped
    command:
      - --entrypoints.web.address=:80
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --providers.docker.network=%s
      - %q
      - --providers.file.directory=%s
      - --providers.file.watch=true
      - --ping=true
    ports:
      - "127.0.0.1:%d:80"
    extra_hosts:
      - "host.docker.internal:%s"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - %q
    networks: [proxy]
    labels:
%s    healthcheck:
      test: ["CMD", "traefik", "healthcheck", "--ping"]
      interval: 2s
      timeout: 2s
      retries: 30
networks:
  proxy:
    name: %q
    external: true
`, c.Image, c.Container, c.Network, "--providers.docker.constraints="+constraint, RoutesMount, s.Port, s.HostAlias,
		s.RoutesDir+":"+RoutesMount+":ro", labels, c.Network)
	return []byte(b.String())
}
