package cli

import (
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
)

// defaultJSONTail is how many lines per service logs --json prints without --tail.
const defaultJSONTail = 100

func newLogsCommand(a *app) *cobra.Command {
	var asJSON, follow bool
	var tail int
	var file string
	cmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Show the logs of the worktree's stack",
		Long: `Show the logs of the worktree's stack, of all services or of the ones named. With
--json it prints the last lines per service (100 without --tail) as one document, with the
values of .env and of the services' environment replaced by ***.`,
		RunE: func(cmd *cobra.Command, services []string) error {
			if asJSON && follow {
				return fail.New(fail.InvalidArguments, "--json and --follow cannot be combined: --json prints one document")
			}
			ctx := cmd.Context()
			st, err := a.findStack(ctx)
			if err != nil {
				return err
			}
			if !asJSON {
				args := []string{"logs"}
				if follow {
					args = append(args, "--follow")
				}
				if tail > 0 {
					args = append(args, "--tail", strconv.Itoa(tail))
				}
				inv := compose.Invocation{Dir: st.wt.Top, Project: st.project}
				return a.opts.Compose.Run(ctx, inv, append(args, services...), cmd.OutOrStdout(), cmd.ErrOrStderr())
			}

			if tail <= 0 {
				tail = defaultJSONTail
			}
			_, p, err := a.loadProject(ctx, st.wt.Top, file, st.project)
			if err != nil {
				return err
			}
			r, err := redactor(st.wt.Top, p)
			if err != nil {
				return err
			}
			cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelProject: st.project})
			if err != nil {
				return err
			}
			grouped := byService(cs)
			names := services
			if len(names) == 0 {
				names = p.ServiceNames()
			}
			doc := logsDocument{header: newHeader(kindLogs), Stack: st.ref(a.shownPort(ctx))}
			for _, name := range names {
				if _, ok := p.Services[name]; !ok {
					return fail.New(fail.InvalidArguments, "the stack has no service %q", name).
						WithHint("its services are %v", p.ServiceNames())
				}
				sl := serviceLogs{Name: name}
				for _, c := range grouped[name] {
					lines, err := a.opts.Docker.Logs(ctx, c.ID, tail)
					if err != nil {
						return err
					}
					sl.Lines = append(sl.Lines, r.Lines(lines)...)
				}
				doc.Services = append(doc.Services, sl)
			}
			slices.SortFunc(doc.Services, func(x, y serviceLogs) int { return strings.Compare(x.Name, y.Name) })
			return printJSON(cmd.OutOrStdout(), doc)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing new lines")
	cmd.Flags().IntVar(&tail, "tail", 0, "only the last n lines of each service")
	cmd.Flags().StringVar(&file, "file", "", "compose file (default: compose.yaml or another name Compose looks for)")
	registerJSON(cmd, &asJSON)
	return cmd
}
