package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
)

func newOpenCommand(a *app) *cobra.Command {
	var asJSON, printOnly bool
	var file string
	cmd := &cobra.Command{
		Use:   "open [service]",
		Short: "Open the stack's address, or a service's, in the browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := a.findStack(ctx)
			if err != nil {
				return err
			}
			port := a.shownPort(ctx)
			doc := addressDocument{header: newHeader(kindAddress), URL: address(st.id.Host(), port, "/")}
			if len(args) == 1 {
				_, p, err := a.loadProject(ctx, st.wt.Top, file, st.project)
				if err != nil {
					return err
				}
				routes, _, err := stack.Routes(p)
				if err != nil {
					return err
				}
				if _, ok := stack.Find(routes, args[0]); !ok {
					names := make([]string, len(routes))
					for i, r := range routes {
						names[i] = r.Service
					}
					return fail.New(fail.InvalidArguments, "the stack has no HTTP service %q", args[0]).
						WithHint("its HTTP services are: %s", strings.Join(names, ", "))
				}
				doc.Service = args[0]
				doc.URL = address(st.id.ServiceHost(args[0]), port, "/")
			}
			switch {
			case asJSON:
				return printJSON(cmd.OutOrStdout(), doc)
			case printOnly:
				_, err := fmt.Fprintln(cmd.OutOrStdout(), doc.URL)
				return err
			}
			if err := a.opts.Browser.Open(doc.URL); err != nil {
				return fail.New(fail.Internal, "open %s: %v", doc.URL, err).WithHint("open it yourself, or print it with --print")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), doc.URL)
			return err
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the address instead of opening it")
	cmd.Flags().StringVarP(&file, "file", "f", "", "compose file (default: compose.yaml or another name Compose looks for)")
	registerJSON(cmd, &asJSON)
	return cmd
}

// SystemBrowser opens URLs with the platform's opener: open on macOS, $BROWSER (set by VS Code
// in a devcontainer) or xdg-open on Linux.
type SystemBrowser struct {
	Getenv func(string) string
}

// Open opens url.
func (b SystemBrowser) Open(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Run()
	}
	if browser := b.Getenv("BROWSER"); browser != "" {
		return exec.Command(browser, url).Run()
	}
	return exec.Command("xdg-open", url).Run()
}
