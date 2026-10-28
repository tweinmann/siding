package stack

import (
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

var withCompose = flag.Bool("compose", false, "also check that docker compose accepts every golden override")

func svc(ports []int, labels map[string]string) compose.Service {
	if labels == nil {
		labels = map[string]string{}
	}
	return compose.Service{Ports: ports, Labels: labels, Networks: []string{"default"}}
}

func TestRoutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		services map[string]compose.Service
		routes   []Route
		warnings int
		code     string
	}{
		{
			name: "root and api",
			services: map[string]compose.Service{
				"web": svc([]int{3000}, map[string]string{LabelRoute: "/"}),
				"api": svc([]int{8080}, map[string]string{LabelRoute: "/api/"}),
			},
			routes: []Route{{Service: "api", Port: 8080, Path: "/api"}, {Service: "web", Port: 3000, Path: "/"}},
		},
		{
			name:     "one port without label gets a service host",
			services: map[string]compose.Service{"mail": svc([]int{8025}, nil)},
			routes:   []Route{{Service: "mail", Port: 8025}},
		},
		{
			name:     "several ports without label are not routed",
			services: map[string]compose.Service{"debug": svc([]int{3000, 9229}, nil)},
		},
		{
			name:     "several ports with the port label",
			services: map[string]compose.Service{"web": svc([]int{3000, 9229}, map[string]string{LabelPort: "3000", LabelRoute: "/"})},
			routes:   []Route{{Service: "web", Port: 3000, Path: "/"}},
		},
		{
			name:     "port label without ports",
			services: map[string]compose.Service{"web": svc(nil, map[string]string{LabelPort: "80"})},
			routes:   []Route{{Service: "web", Port: 80}},
		},
		{
			name:     "no port",
			services: map[string]compose.Service{"worker": svc(nil, nil)},
		},
		{
			name:     "hidden",
			services: map[string]compose.Service{"admin": svc([]int{80}, map[string]string{LabelRoute: "false"})},
		},
		{
			name:     "two services on one path",
			services: map[string]compose.Service{"web": svc([]int{80}, map[string]string{LabelRoute: "/"}), "admin": svc([]int{80}, map[string]string{LabelRoute: "/"})},
			code:     fail.InvalidRoute,
		},
		{
			name:     "a route without a port",
			services: map[string]compose.Service{"api": svc(nil, map[string]string{LabelRoute: "/api"})},
			code:     fail.InvalidRoute,
		},
		{
			name:     "a route with several ports",
			services: map[string]compose.Service{"web": svc([]int{3000, 9229}, map[string]string{LabelRoute: "/"})},
			code:     fail.AmbiguousPort,
		},
		{
			name:     "not a path",
			services: map[string]compose.Service{"web": svc([]int{80}, map[string]string{LabelRoute: "api"})},
			code:     fail.InvalidRoute,
		},
		{
			name:     "a path that would break the rule",
			services: map[string]compose.Service{"web": svc([]int{80}, map[string]string{LabelRoute: "/a`) || Host(`x"})},
			code:     fail.InvalidRoute,
		},
		{
			name:     "a bad port label",
			services: map[string]compose.Service{"web": svc([]int{80}, map[string]string{LabelPort: "http"})},
			code:     fail.InvalidRoute,
		},
		{
			name:     "network_mode without route warns",
			services: map[string]compose.Service{"tools": {Ports: []int{7000}, Labels: map[string]string{}, NetworkMode: "host"}},
			warnings: 1,
		},
		{
			name:     "network_mode with route fails",
			services: map[string]compose.Service{"tools": {Ports: []int{7000}, Labels: map[string]string{LabelRoute: "/"}, NetworkMode: "host"}},
			code:     fail.InvalidRoute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			routes, warnings, err := Routes(&compose.Project{Services: tt.services})
			if tt.code != "" {
				var e *fail.Error
				if !errors.As(err, &e) || e.Code != tt.code {
					t.Fatalf("err %v, want code %s", err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(routes, tt.routes) || len(warnings) != tt.warnings {
				t.Errorf("routes %+v, warnings %q; want %+v and %d warnings", routes, warnings, tt.routes, tt.warnings)
			}
		})
	}
}

// Each directory in testdata/override holds a captured compose config (config.json, from
// compose.yaml next to it) and the override siding writes for it (override.yml).
func TestOverride(t *testing.T) {
	t.Parallel()
	id := Identity{Repo: "shop", Branch: "main", Worktree: "/src/shop", Machine: "0123456789abcdef", Agent: "claude"}
	for _, dir := range []string{"routing"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join("..", "compose", "testdata", dir)
			data, err := os.ReadFile(filepath.Join(src, "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			p, err := compose.ParseConfig(data)
			if err != nil {
				t.Fatal(err)
			}
			routes, _, err := Routes(p)
			if err != nil {
				t.Fatal(err)
			}
			got := Override(p, id, routes, "siding")
			golden := filepath.Join("testdata", "override", dir+".yml")
			testutil.Golden(t, golden, got)
			if *withCompose {
				checkWithCompose(t, src, golden)
			}
		})
	}
}

// checkWithCompose proves that Compose accepts the override and that the result has no
// published port and keeps the user's networks.
func checkWithCompose(t *testing.T, src, override string) {
	t.Helper()
	abs, _ := filepath.Abs(override)
	cmd := exec.Command("docker", "compose", "-p", "shop-main", "-f", "compose.yaml", "-f", abs, "config", "--format", "json")
	cmd.Dir = src
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker compose config with the override: %v", err)
	}
	merged, err := compose.ParseConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"published"`) {
		t.Error("a port is still published")
	}
	for name, s := range merged.Services {
		if s.Labels[LabelRepo] != "shop" {
			t.Errorf("%s lacks the repo label", name)
		}
	}
	if got := merged.Services["api"].Networks; !reflect.DeepEqual(got, []string{"backend", "default", ProxyNetworkKey}) {
		t.Errorf("api networks %v", got)
	}
	if got := merged.Services["worker"].Networks; !reflect.DeepEqual(got, []string{"default"}) {
		t.Errorf("worker networks %v", got)
	}
}
