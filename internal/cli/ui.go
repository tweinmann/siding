package cli

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/overview"
	"github.com/tweinmann/siding/internal/stack"
)

//go:embed ui/page.html
var pageHTML string

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"bytes": bytesText,
}).Parse(pageHTML))

type pageData struct {
	Stacks       stackListDocument
	GC           gcDocument
	Used, Budget string
	// Percent is the memory in use against the budget; Bar is the same, at most 100.
	Percent, Bar int
}

func renderPage(stacks stackListDocument, gc gcDocument) ([]byte, error) {
	d := pageData{Stacks: stacks, GC: gc, Used: bytesText(stacks.MemoryBytes), Budget: bytesText(uint64(stacks.BudgetBytes))}
	if stacks.BudgetBytes > 0 {
		d.Percent = int(stacks.MemoryBytes * 100 / uint64(stacks.BudgetBytes))
	}
	d.Bar = min(d.Percent, 100)
	var b bytes.Buffer
	if err := pageTemplate.Execute(&b, d); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// overviewHandler serves the page and its JSON endpoints, all read-only: the page at /, and at
// /api/stacks and /api/gc the documents of ps --json and gc --dry-run --json. The mux answers
// other methods with 405 and other paths with 404.
func (a *app) overviewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		stacks, err := a.ps(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		gc, err := a.gcDryRun(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page, err := renderPage(stacks, gc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	serveJSON := func(get func(context.Context) (any, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			doc, err := get(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = printJSON(w, doc)
		}
	}
	mux.HandleFunc("GET /api/stacks", serveJSON(func(ctx context.Context) (any, error) { return a.ps(ctx) }))
	mux.HandleFunc("GET /api/gc", serveJSON(func(ctx context.Context) (any, error) { return a.gcDryRun(ctx) }))
	return mux
}

// gcDryRun is the document of gc --dry-run --json.
func (a *app) gcDryRun(ctx context.Context) (gcDocument, error) {
	doc, _, err := a.gcPlan(ctx)
	doc.DryRun = true
	return doc, err
}

func newUICommand(a *app) *cobra.Command {
	var asJSON bool
	var port int
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the overview at http://siding.localhost while it runs",
		Long: `Serve a read-only overview of every stack, the memory budget and what gc would remove, at
http://siding.localhost through siding-proxy, until Ctrl-C. It listens on this machine only and
removes its route when it stops; nothing of it keeps running.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.ui(cmd, port, asJSON)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "local port (default: a free one)")
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) ui(cmd *cobra.Command, port int, asJSON bool) error {
	ctx := cmd.Context()
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return err
	}
	proxyPort, err := a.proxyPort(ctx)
	if err != nil {
		return err
	}
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return err
	}
	alias := proxyContainer.Labels[stack.LabelHostAlias]
	if alias == "" {
		return fail.New(fail.ProxyNotRunning, "%s has no route to the overview yet", a.opts.Proxy.Container).
			WithHint("update it with siding setup")
	}

	// The first address picks the port; the others take the same one.
	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for _, host := range overview.ListenAddrs(alias) {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return fail.New(fail.PortInUse, "siding ui cannot listen on %s:%d: %v", host, port, err).
				WithHint("choose another port with --port, or run siding setup if the proxy network changed")
		}
		listeners = append(listeners, l)
		port = l.Addr().(*net.TCPAddr).Port
	}

	routes := filepath.Join(a.opts.Home, ".siding", "proxy", a.opts.Proxy.Project, "routes")
	removeRoute, err := overview.WriteRoute(routes, port)
	if err != nil {
		return err
	}
	defer func() { _ = removeRoute() }()

	url := address(overview.Host, proxyPort, "/")
	if asJSON {
		if err := printJSON(cmd.OutOrStdout(), addressDocument{header: newHeader(kindAddress), URL: url}); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(cmd.OutOrStdout(), "siding ui at %s (and http://127.0.0.1:%d/); Ctrl-C stops it\n", url, port); err != nil {
		return err
	}

	srv := &http.Server{Handler: a.overviewHandler(), ReadHeaderTimeout: 10 * time.Second}
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func(l net.Listener) { errs <- srv.Serve(l) }(l)
	}
	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("siding ui: %w", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return ctx.Err()
}
