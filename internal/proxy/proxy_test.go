package proxy

import (
	"path/filepath"
	"testing"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestComposeFile(t *testing.T) {
	t.Parallel()
	idle := int64(900)
	for _, tt := range []struct {
		name string
		s    Settings
	}{
		{name: "defaults", s: Settings{Port: 80, Machine: "not-a-real-machine", RoutesDir: "/home/u/.siding/proxy/siding/routes", HostAlias: "172.19.0.1"}},
		{name: "settings", s: Settings{Port: 8080, Machine: "not-a-real-machine", Budget: 12 << 30, Builds: 2, Idle: &idle,
			RoutesDir: "/home/u/.siding/proxy/siding/routes", HostAlias: HostGateway}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Golden(t, filepath.Join("testdata", "compose-"+tt.name+".yml"), ComposeFile(Default(), tt.s))
		})
	}
}
