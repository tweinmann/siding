package docker

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

func TestFromInspect(t *testing.T) {
	t.Parallel()
	port, err := network.ParsePort("80/tcp")
	if err != nil {
		t.Fatal(err)
	}
	in := container.InspectResponse{
		ID:           "abc",
		Name:         "/siding-proxy",
		RestartCount: 2,
		Config:       &container.Config{Image: "traefik:v3.7.16", Labels: map[string]string{"dev.siding.machine": "m"}},
		State: &container.State{
			Status:   container.StateRunning,
			ExitCode: 0,
			Health: &container.Health{Status: container.Unhealthy, Log: []*container.HealthcheckResult{
				{Output: "old"}, {Output: "connection refused\n"},
			}},
		},
		NetworkSettings: &container.NetworkSettings{Ports: network.PortMap{
			port: {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "8080"}},
		}},
	}
	want := Container{
		ID: "abc", Name: "siding-proxy", Image: "traefik:v3.7.16", Labels: map[string]string{"dev.siding.machine": "m"},
		State: "running", Health: "unhealthy", HealthOutput: "connection refused", RestartCount: 2,
		Ports: []Port{{HostIP: "127.0.0.1", HostPort: 8080, ContainerPort: 80}},
	}
	if got := fromInspect(in); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got := fromInspect(container.InspectResponse{}); got.Labels == nil {
		t.Error("labels must never be nil")
	}
}

func TestMemoryUsage(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		usage uint64
		stats map[string]uint64
		want  uint64
	}{
		{name: "cgroup v2", usage: 1000, stats: map[string]uint64{"inactive_file": 300}, want: 700},
		{name: "cgroup v1", usage: 1000, stats: map[string]uint64{"total_inactive_file": 100}, want: 900},
		{name: "no stats", usage: 1000, want: 1000},
		{name: "cache larger than usage", usage: 10, stats: map[string]uint64{"inactive_file": 20}, want: 10},
	} {
		if got := MemoryUsage(tt.usage, tt.stats); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestHelperOptions(t *testing.T) {
	t.Parallel()
	h := Helper{Image: "busybox:1.37.0", Binds: []string{"shop-main_data:/from:ro", "siding-snap-shop-base-data:/to"},
		Cmd: []string{"cp", "-a", "/from/.", "/to/"}, Labels: map[string]string{"dev.siding.machine": "m"}}
	o := helperOptions(h)
	if o.Config.Image != h.Image || !reflect.DeepEqual(o.Config.Cmd, h.Cmd) || !reflect.DeepEqual(o.Config.Labels, h.Labels) {
		t.Errorf("config %+v", o.Config)
	}
	// No network: a copy needs none, and nothing in it can reach out.
	if !reflect.DeepEqual(o.HostConfig.Binds, h.Binds) || o.HostConfig.NetworkMode != "none" {
		t.Errorf("host config %+v", o.HostConfig)
	}
}
