package lint

import (
	"os"
	"path/filepath"
	"reflect"
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
