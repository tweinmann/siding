package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/budget"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/queue"
	"github.com/tweinmann/siding/internal/stack"
)

func newPsCommand(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List every stack on this machine with its state, address and memory",
		Long: `List every stack on this machine, from any directory: repository, branch, state
(running, partial or stopped), address, memory in use and worktree. ps reads only Docker; the
full verdict on one stack is siding status.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.ps(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			return printPsText(cmd, doc)
		},
	}
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) ps(ctx context.Context) (stackListDocument, error) {
	doc := stackListDocument{header: newHeader(kindStackList)}
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return doc, err
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return doc, err
	}
	port := a.shownPort(ctx)
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return doc, err
	}
	if doc.BudgetBytes, err = a.budget(ctx, proxyContainer); err != nil {
		return doc, err
	}
	projects := map[string][]docker.Container{}
	for _, c := range cs {
		p := c.Labels[stack.LabelProject]
		projects[p] = append(projects[p], c)
	}
	memory, err := a.memory(ctx, cs)
	if err != nil {
		return doc, err
	}
	for project, containers := range projects {
		id := stack.FromLabels(containers[0].Labels)
		e := stackEntry{Project: project, Repo: id.Repo, Branch: id.Branch, Worktree: id.Worktree,
			Machine: id.Machine, Agent: id.Agent, URL: address(id.Host(), port, "/")}
		running := 0
		for _, c := range containers {
			if c.State == "running" {
				running++
			}
			e.MemoryBytes += memory[c.ID]
		}
		switch running {
		case len(containers):
			e.State = "running"
		case 0:
			e.State = "stopped"
		default:
			e.State = "partial"
		}
		if _, err := os.Stat(id.Worktree); errors.Is(err, fs.ErrNotExist) {
			e.WorktreeMissing = true
		}
		doc.MemoryBytes += e.MemoryBytes
		doc.Stacks = append(doc.Stacks, e)
	}
	sort.Slice(doc.Stacks, func(i, j int) bool { return doc.Stacks[i].Project < doc.Stacks[j].Project })
	line, err := queue.Open(a.opts.Home, a.opts.Alive).List()
	if err != nil {
		return doc, err
	}
	for i, w := range line {
		doc.Waiting = append(doc.Waiting, waitingEntry{Project: w.Project, Place: i + 1, MemoryBytes: w.Estimate,
			Since: w.Since.UTC().Format(time.RFC3339)})
	}
	return doc, nil
}

// memory asks for the memory of every running container at once; a stats call takes a moment.
func (a *app) memory(ctx context.Context, cs []docker.Container) (map[string]uint64, error) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := map[string]uint64{}
	var firstErr error
	for _, c := range cs {
		if c.State != "running" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			m, err := a.opts.Docker.Memory(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[id] = m
		}(c.ID)
	}
	wg.Wait()
	a.learnPeaks(cs, out)
	return out, firstErr
}

// learnPeaks keeps the highest memory per repository and service in the losable cache; a
// cache that cannot be written is not worth failing a command for.
func (a *app) learnPeaks(cs []docker.Container, memory map[string]uint64) {
	samples := budget.Peaks{}
	for _, c := range cs {
		m, ok := memory[c.ID]
		repo, service := c.Labels[stack.LabelRepo], c.Labels[stack.LabelService]
		if !ok || repo == "" || service == "" {
			continue
		}
		if k := budget.PeakKey(repo, service); int64(m) > samples[k] {
			samples[k] = int64(m)
		}
	}
	_ = budget.Learn(a.opts.Home, samples)
}

func printPsText(cmd *cobra.Command, doc stackListDocument) error {
	if len(doc.Stacks) == 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "No stacks. Start one with siding up in a worktree. Memory budget %s.\n%s", bytesText(uint64(doc.BudgetBytes)), waitingText(doc.Waiting))
		return err
	}
	rows := [][]string{{"REPO", "BRANCH", "STATE", "URL", "MEMORY", "WORKTREE"}}
	for _, s := range doc.Stacks {
		worktree := s.Worktree
		if s.WorktreeMissing {
			worktree += " (missing)"
		}
		rows = append(rows, []string{s.Repo, s.Branch, s.State, s.URL, bytesText(s.MemoryBytes), worktree})
	}
	text := fmt.Sprintf("%stotal: %s of %s\n", table("", rows), bytesText(doc.MemoryBytes), bytesText(uint64(doc.BudgetBytes)))
	_, err := fmt.Fprint(cmd.OutOrStdout(), text+waitingText(doc.Waiting))
	return err
}

func waitingText(waiting []waitingEntry) string {
	if len(waiting) == 0 {
		return ""
	}
	rows := [][]string{}
	for _, w := range waiting {
		rows = append(rows, []string{fmt.Sprintf("%d.", w.Place), w.Project, "needs " + bytesText(uint64(w.MemoryBytes)), "since " + w.Since})
	}
	return "waiting for memory:\n" + table("  ", rows)
}

// bytesText shows memory in MiB, or GiB from 1 GiB on; less than half a MiB, but not nothing, is
// "<1 MiB".
func bytesText(n uint64) string {
	const mib, gib = 1 << 20, 1 << 30
	if n > 0 && n < mib/2 {
		return "<1 MiB"
	}
	if n >= gib {
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	}
	return fmt.Sprintf("%d MiB", (n+mib/2)/mib)
}
