package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestOverviewHandler(t *testing.T) {
	t.Parallel()
	e := gcEnv(t)
	e.proxyUp(80)
	a := &app{opts: e.opts}
	srv := httptest.NewServer(a.overviewHandler())
	t.Cleanup(srv.Close)

	get := func(method, path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, e.clean(string(body))
	}

	for path, args := range map[string][]string{"/api/stacks": {"ps", "--json"}, "/api/gc": {"gc", "--dry-run", "--json"}} {
		code, body := get(http.MethodGet, path)
		_, want, _ := e.run(args...)
		if code != 200 || body != want {
			t.Errorf("GET %s: %d\n%s\nwant the output of siding %s:\n%s", path, code, body, strings.Join(args, " "), want)
		}
	}
	if code, body := get(http.MethodGet, "/"); code != 200 || !strings.Contains(body, "shop-done") || !strings.Contains(body, `http-equiv="refresh"`) {
		t.Errorf("GET /: %d\n%s", code, body)
	}
	for _, m := range []string{http.MethodPost, http.MethodDelete} {
		if code, _ := get(m, "/api/gc"); code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/gc: %d, want 405", m, code)
		}
	}
	if code, _ := get(http.MethodGet, "/nope"); code != http.StatusNotFound {
		t.Errorf("GET /nope: %d", code)
	}
	if len(e.compose.recorded()) != 0 {
		t.Error("the overview ran docker compose")
	}
}

// The page for the documents of the golden files stacklist.json and gc-dryrun.json.
func TestRenderPage(t *testing.T) {
	t.Parallel()
	var stacks stackListDocument
	var gc gcDocument
	for file, doc := range map[string]any{"stacklist.json": &stacks, "gc-dryrun.json": &gc} {
		data, err := os.ReadFile(filepath.Join("testdata", "json", file))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, doc); err != nil {
			t.Fatal(err)
		}
	}
	page, err := renderPage(stacks, gc)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, filepath.Join("testdata", "ui", "page.html"), page)

	empty, err := renderPage(stackListDocument{BudgetBytes: 1 << 30}, gcDocument{})
	if err != nil || !strings.Contains(string(empty), "No stacks.") || !strings.Contains(string(empty), "0 MiB of 1.0 GiB (0 %)") {
		t.Errorf("empty page: %v\n%s", err, empty)
	}
}

func TestUI(t *testing.T) {
	t.Parallel()

	t.Run("serves until cancelled", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.opts.IsLocalAddress = func(string) bool { return false } // loopback only, as with Docker Desktop
		setupDoc(t, e)
		route := filepath.Join(e.opts.Home, ".siding", "proxy", "siding", "routes", "ui.yml")

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan int, 1)
		var out, errOut bytes.Buffer
		go func() { done <- Execute(ctx, New(e.opts), []string{"ui"}, &out, &errOut) }()

		var data []byte
		for i := 0; i < 200 && len(data) == 0; i++ {
			time.Sleep(10 * time.Millisecond)
			data, _ = os.ReadFile(route)
		}
		m := regexp.MustCompile(`host\.docker\.internal:(\d+)`).FindSubmatch(data)
		if m == nil {
			cancel()
			t.Fatalf("no route file: %q", data)
		}
		resp, err := http.Get("http://127.0.0.1:" + string(m[1]) + "/api/stacks")
		if err != nil || resp.StatusCode != 200 {
			cancel()
			t.Fatalf("GET /api/stacks: %v %v", resp, err)
		}
		_ = resp.Body.Close()

		cancel()
		if code := <-done; code != exitInterrupted || errOut.Len() != 0 {
			t.Errorf("exit %d, stderr %q", code, errOut.String())
		}
		if _, err := os.Stat(route); !os.IsNotExist(err) {
			t.Errorf("the route survived: %v", err)
		}
		if !strings.Contains(out.String(), "siding ui at http://siding.localhost/ (and http://127.0.0.1:"+string(m[1])+"/)") {
			t.Errorf("stdout %q", out.String())
		}
	})

	t.Run("a proxy of siding v0.1", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		if code, _, stderr := e.run("ui"); code != exitFailed || !strings.Contains(stderr, "siding setup") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("no proxy", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if code, stdout, _ := e.run("ui", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "ProxyNotRunning"`) {
			t.Errorf("exit %d, stdout %s", code, stdout)
		}
	})
}
