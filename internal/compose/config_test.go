package compose

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
)

// testdata/routing/config.json is what `docker compose -p shop-main config --format json`
// prints for testdata/routing/compose.yaml with the Compose pinned in the devcontainer. After a
// Compose upgrade, capture it again in that directory.
func TestParseConfig(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "routing", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "shop-main" {
		t.Errorf("name %q", p.Name)
	}
	if got, want := p.ServiceNames(), []string{"admin", "api", "db", "mail", "tools", "web", "worker"}; !reflect.DeepEqual(got, want) {
		t.Errorf("services %v, want %v", got, want)
	}
	services := []struct {
		name        string
		ports       []int
		networks    []string
		networkMode string
		route       string
	}{
		{name: "web", ports: []int{3000}, networks: []string{"default"}, route: "/"},
		{name: "api", ports: []int{8080, 9229}, networks: []string{"backend", "default"}, route: "/api"},
		{name: "mail", ports: []int{8025}, networks: []string{"default"}},
		{name: "admin", ports: []int{80}, networks: []string{"default"}, route: "false"},
		{name: "db", ports: []int{5432}, networks: []string{"backend"}},
		{name: "worker", networks: []string{"default"}},
		{name: "tools", ports: []int{7000, 7001}, networkMode: "host"},
	}
	for _, want := range services {
		s := p.Services[want.name]
		if !reflect.DeepEqual(s.Ports, want.ports) || !reflect.DeepEqual(s.Networks, want.networks) ||
			s.NetworkMode != want.networkMode || s.Labels["dev.siding.route"] != want.route {
			t.Errorf("%s: ports %v, networks %v, network_mode %q, route %q; want %v, %v, %q, %q", want.name,
				s.Ports, s.Networks, s.NetworkMode, s.Labels["dev.siding.route"], want.ports, want.networks, want.networkMode, want.route)
		}
	}
	if got := p.Services["api"].Environment["API_TOKEN"]; got != "not-a-real-api-token" {
		t.Errorf("env_file value %q, want it inlined into environment", got)
	}
	if got := p.Services["web"].Environment["DB_URL"]; got != "postgres://app:not-a-real-token@db/app" {
		t.Errorf("interpolated value %q", got)
	}
	if got := p.Volumes["shared"]; !got.External || got.Name != "shared-cache" {
		t.Errorf("external volume %+v", got)
	}
	if got := p.Volumes["data"]; got.External || got.Name != "shop-main_data" {
		t.Errorf("volume %+v", got)
	}
	if got := p.Networks["backend"]; got.External || got.Name != "shop-main_backend" {
		t.Errorf("network %+v", got)
	}
}

func TestFindFile(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	if _, err := FindFile(top, top, ""); codeOf(err) != fail.ComposeFileNotFound {
		t.Fatalf("empty worktree: %v", err)
	}
	for _, name := range []string{"docker-compose.yml", "compose.yaml"} {
		if err := os.WriteFile(filepath.Join(top, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := FindFile(top, top, ""); err != nil || got != filepath.Join(top, "compose.yaml") {
		t.Errorf("got %q, %v; want compose.yaml first", got, err)
	}
	sub := filepath.Join(top, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FindFile(sub, top, "../docker-compose.yml"); err != nil || got != filepath.Join(top, "docker-compose.yml") {
		t.Errorf("--file relative to the current directory: %q, %v", got, err)
	}
	if _, err := FindFile(top, top, "nope.yml"); codeOf(err) != fail.ComposeFileNotFound {
		t.Errorf("missing --file: %v", err)
	}
}

func TestAtLeast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		have string
		want bool
	}{{"2.24.0", true}, {"2.24", true}, {"2.23.9", false}, {"5.6.1", true}, {"2.3", false}, {"2.100", true}} {
		if got := AtLeast(tt.have, MinVersion); got != tt.want {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", tt.have, MinVersion, got, tt.want)
		}
	}
}

func TestTailBuffer(t *testing.T) {
	t.Parallel()
	b := &tailBuffer{max: 2}
	_, _ = b.Write([]byte("one\ntwo\nthr"))
	_, _ = b.Write([]byte("ee\nfour"))
	if got := b.String(); got != "three\nfour" {
		t.Errorf("got %q", got)
	}
}

func codeOf(err error) string {
	var e *fail.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// testdata/lint/config.json is captured like testdata/routing/config.json, from
// testdata/lint/compose.yaml, with the capturing directory replaced by /src/shop.
func TestParseConfigDetails(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "lint", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	const mib = 1 << 20
	tests := []struct {
		name          string
		containerName string
		binds         []string
		published     []int
		healthcheck   bool
		memoryLimit   int64
		replicas      int
	}{
		{name: "web", healthcheck: true, memoryLimit: 512 * mib, replicas: 2},
		{name: "db", containerName: "shop-db", binds: []string{"/var/lib/shop", "/src/shop/init"}, published: []int{5432}, memoryLimit: 1024 * mib, replicas: 1},
		{name: "worker", replicas: 3},
		{name: "tools", replicas: 1},
	}
	for _, want := range tests {
		s := p.Services[want.name]
		if s.ContainerName != want.containerName || !reflect.DeepEqual(s.Binds, want.binds) || !reflect.DeepEqual(s.Published, want.published) ||
			s.Healthcheck != want.healthcheck || s.MemoryLimit != want.memoryLimit || s.Replicas != want.replicas {
			t.Errorf("%s: %+v\nwant %+v", want.name, s, want)
		}
	}
	if got := p.Volumes["cache"]; got.Name != "pip-cache" || got.Labels["dev.siding.shared"] != "true" {
		t.Errorf("volume cache %+v", got)
	}
	if got := p.Volumes["pgdata"]; got.Name != "shop-pgdata" || len(got.Labels) != 0 {
		t.Errorf("volume pgdata %+v", got)
	}
	if got := p.Networks["shared"]; got.Name != "everyone" {
		t.Errorf("network shared %+v", got)
	}
}

func TestErrorLines(t *testing.T) {
	t.Parallel()
	out := ` Container demo-db Creating 
 service:db:1 Error response from daemon: Conflict. The container name "/demo-db" is already in use. 
Error response from daemon: Conflict. The container name "/demo-db" is already in use.
`
	if got, want := errorLines(out), `Error response from daemon: Conflict. The container name "/demo-db" is already in use.`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	pull := " traefik Pulling \n traefik Error Get \"https://registry-1.docker.io/v2/\": net/http: TLS handshake timeout\n" +
		"Error response from daemon: Get \"https://registry-1.docker.io/v2/\": net/http: TLS handshake timeout\n"
	if got := errorLines(pull); strings.Contains(got, "Pulling") || !strings.Contains(got, "TLS handshake timeout") {
		t.Errorf("a failed pull: %q", got)
	}
	if got := errorLines(" Network x_default Creating\nno such service: nope\n"); got != "no such service: nope" {
		t.Errorf("without an error line: %q", got)
	}
}
