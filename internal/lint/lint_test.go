package lint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/compose"
)

func TestCheckEveryRule(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "compose", "testdata", "lint", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := compose.ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ rule, severity, subject string }
	var got []key
	for _, f := range Check(p, "shop-main", "/src/shop") {
		if f.Message == "" {
			t.Errorf("%s without a message", f.Rule)
		}
		got = append(got, key{f.Rule, f.Severity, f.Subject})
	}
	want := []key{
		{ContainerName, Error, "service db"},
		{FixedNetworkName, Error, "network shared"},
		{FixedVolumeName, Error, "volume pgdata"},
		{HostPath, Warning, "service db"}, // /var/lib/shop; ./init is inside the worktree
		{NoHealthcheck, Warning, "service db"},
		{NetworkMode, Warning, "service tools"},
		{NoHealthcheck, Warning, "service tools"},
		{LocalhostURL, Warning, "service worker"}, // API_URL; DATABASE_URL names db, not localhost
		{NoHealthcheck, Warning, "service worker"},
		{PublishedPort, Info, "service db"},
		{FixedVolumeName, Info, "volume cache"}, // shared on purpose
		{NoRoot, Info, "stack"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings\n%v\nwant\n%v", got, want)
	}
	if !HasErrors(Check(p, "shop-main", "/src/shop")) {
		t.Error("HasErrors is false")
	}
}

func TestCheckClean(t *testing.T) {
	t.Parallel()
	p := &compose.Project{
		Services: map[string]compose.Service{
			"web": {Ports: []int{3000}, Labels: map[string]string{"dev.siding.route": "/"}, Networks: []string{"default"}, Healthcheck: true},
			"db":  {Labels: map[string]string{}, Networks: []string{"default"}, Healthcheck: true, Binds: []string{"/src/shop/init"}},
		},
		Networks: map[string]compose.Resource{"default": {Name: "shop-main_default"}, "ext": {Name: "shared", External: true}},
		Volumes:  map[string]compose.Resource{"data": {Name: "shop-main_data"}},
	}
	if fs := Check(p, "shop-main", "/src/shop"); len(fs) != 0 {
		t.Errorf("findings %+v", fs)
	}
}

func TestCheckRoutes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		svc  compose.Service
		rule string
	}{
		{name: "invalid", svc: compose.Service{Ports: []int{80}, Labels: map[string]string{"dev.siding.route": "api"}, Healthcheck: true}, rule: InvalidRoute},
		{name: "ambiguous", svc: compose.Service{Ports: []int{3000, 9229}, Labels: map[string]string{"dev.siding.route": "/"}, Healthcheck: true}, rule: AmbiguousPort},
	} {
		p := &compose.Project{Services: map[string]compose.Service{"web": tt.svc}}
		fs := Check(p, "shop-main", "/src/shop")
		if len(fs) != 1 || fs[0].Rule != tt.rule || fs[0].Severity != Error {
			t.Errorf("%s: %+v", tt.name, fs)
		}
	}
}

func TestLocalhostURL(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, value, want string // want is host:port, "" for no finding
	}{
		{name: "an API URL", value: "http://localhost:3000/v1", want: "localhost:3000"},
		{name: "a password in the value", value: "postgres://app:not-a-real-token@127.0.0.1:5432/app", want: "127.0.0.1:5432"},
		{name: "IPv6", value: "http://[::1]:8080", want: "[::1]:8080"},
		{name: "upper case", value: "HTTP://LOCALHOST:80", want: "LOCALHOST:80"},
		{name: "without a port", value: "localhost"},
		{name: "another host", value: "http://api:3000"},
		{name: "a longer name", value: "http://mylocalhost:3000"},
	} {
		p := &compose.Project{Services: map[string]compose.Service{"web": {Environment: map[string]string{"X_URL": tt.value}}}}
		var found []Finding
		for _, f := range Check(p, "shop-main", "/src/shop") {
			if f.Rule == LocalhostURL {
				found = append(found, f)
			}
		}
		switch {
		case tt.want == "" && len(found) > 0:
			t.Errorf("%s: unexpected %+v", tt.name, found)
		case tt.want != "" && len(found) != 1:
			t.Errorf("%s: %d findings", tt.name, len(found))
		case tt.want != "":
			f := found[0]
			if f.Severity != Warning || f.Subject != "service web" || !strings.Contains(f.Message, "X_URL") || !strings.Contains(f.Message, tt.want) {
				t.Errorf("%s: %+v", tt.name, f)
			}
			if strings.Contains(f.Message+f.Hint, "not-a-real-token") || strings.Contains(f.Message+f.Hint, "/v1") {
				t.Errorf("%s: the finding holds more of the value than host and port: %+v", tt.name, f)
			}
		}
	}
}
