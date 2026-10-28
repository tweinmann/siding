package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		port int
		args []string
		want string
	}{
		{name: "port 80", port: 80, args: []string{"open", "--print"}, want: "http://shop.main.localhost/\n"},
		{name: "port 8080", port: 8080, args: []string{"open", "--print"}, want: "http://shop.main.localhost:8080/\n"},
		{name: "a service", port: 8080, args: []string{"open", "--print", "mail"}, want: "http://mail.shop.main.localhost:8080/\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.proxyUp(tt.port)
			e.stackUp(e.id(), "web")
			if code, stdout, _ := e.run(tt.args...); code != exitOK || stdout != tt.want || len(e.browser.opened) != 0 {
				t.Errorf("exit %d, stdout %q, opened %v; want %q and no browser", code, stdout, e.browser.opened, tt.want)
			}
		})
	}

	t.Run("opens the browser", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "web")
		if code, _, _ := e.run("open"); code != exitOK || !slices.Equal(e.browser.opened, []string{"http://shop.main.localhost/"}) {
			t.Errorf("exit %d, opened %v", code, e.browser.opened)
		}
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "web")
		code, stdout, _ := e.run("open", "api", "--json")
		if code != exitOK || len(e.browser.opened) != 0 {
			t.Fatalf("exit %d, opened %v", code, e.browser.opened)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "address.json"), []byte(stdout))
	})

	t.Run("unknown service", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "web")
		code, _, stderr := e.run("open", "nope")
		if code != exitFailed || !strings.Contains(stderr, `no HTTP service "nope"`) || !strings.Contains(stderr, "api, mail, web") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})
}
