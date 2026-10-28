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

// ComposeFile renders the proxy's compose file. The constraint keeps Traefik to containers
// siding labelled for this proxy's network, so foreign Traefik labels on the same daemon are
// never exposed.
func ComposeFile(c Config, port int, machine string) []byte {
	constraint := fmt.Sprintf("LabelRegex(`dev.siding.repo`, `.+`) && Label(`traefik.docker.network`, `%s`)", c.Network)
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
      - --ping=true
    ports:
      - "127.0.0.1:%d:80"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks: [proxy]
    labels:
      dev.siding.machine: %q
    healthcheck:
      test: ["CMD", "traefik", "healthcheck", "--ping"]
      interval: 2s
      timeout: 2s
      retries: 30
networks:
  proxy:
    name: %q
    external: true
`, c.Image, c.Container, c.Network, "--providers.docker.constraints="+constraint, port, machine, c.Network)
	return []byte(b.String())
}
