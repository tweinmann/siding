package overview

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriteRoute(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A route a killed siding ui left behind.
	if err := os.WriteFile(filepath.Join(dir, RouteFileName), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	remove, err := WriteRoute(dir, 4711)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, RouteFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rule: \"Host(`siding.localhost`)\"", `url: "http://host.docker.internal:4711"`, "entrypoints: [web]"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("route lacks %s:\n%s", want, data)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}
	if err := remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, RouteFileName)); !os.IsNotExist(err) {
		t.Errorf("the route is still there: %v", err)
	}
	if err := remove(); err != nil {
		t.Errorf("removing twice: %v", err)
	}
}

func TestListenAddrs(t *testing.T) {
	t.Parallel()
	for alias, want := range map[string][]string{
		"172.19.0.1":   {"127.0.0.1", "172.19.0.1"},
		"host-gateway": {"127.0.0.1"},
		"":             {"127.0.0.1"},
	} {
		if got := ListenAddrs(alias); !reflect.DeepEqual(got, want) {
			t.Errorf("ListenAddrs(%q) = %v, want %v", alias, got, want)
		}
	}
}
