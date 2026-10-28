// Package gc decides which stacks can go: a pure function from what Docker and git know about
// each stack to the actions siding gc takes. It never removes git worktrees or branches, and
// never anything that could hold unmerged or uncommitted work.
package gc

import (
	"sort"

	"github.com/tweinmann/siding/internal/git"
)

// Reasons a stack goes. Part of the --json contract.
const (
	Merged       = "Merged"
	WorktreeGone = "WorktreeGone"
)

// Reasons a stack stays. Part of the --json contract.
const (
	Dirty         = "Dirty"
	Unmerged      = "Unmerged"
	DefaultBranch = "DefaultBranch"
)

// Facts are what siding found out about one stack.
type Facts struct {
	Project  string
	Branch   string
	Worktree string
	// Machine is the stack's dev.siding.machine; gc only acts on this machine's stacks.
	Machine string
	// WorktreeExists is false when the worktree directory is gone.
	WorktreeExists bool
	// Dirty: the worktree has changes. Only meaningful while it exists.
	Dirty bool
	// Merge is the branch's merge state; MergeKnown is false when git could not be asked (no
	// dev.siding.gitdir label, or the git directory is gone too).
	Merge      git.MergeState
	MergeKnown bool
}

// Action removes a stack.
type Action struct {
	Project string
	Reason  string
	// Volumes: also remove the volumes, and with them the data.
	Volumes bool
}

// Kept is a stack gc leaves alone.
type Kept struct {
	Project string
	Reason  string
}

// Plan returns what gc removes and what it keeps of the stacks of machine, each sorted by
// project. Stacks of other machines are neither removed nor listed.
func Plan(stacks []Facts, machine string) (actions []Action, kept []Kept) {
	for _, f := range stacks {
		if f.Machine != machine {
			continue
		}
		merged := f.MergeKnown && f.Merge.MergedForGC()
		switch {
		case !f.WorktreeExists:
			// The worktree is gone: nothing can start these containers again. The data goes
			// only when git confirms the branch's work is merged.
			actions = append(actions, Action{Project: f.Project, Reason: WorktreeGone, Volumes: merged})
		case f.Dirty:
			kept = append(kept, Kept{Project: f.Project, Reason: Dirty})
		case f.MergeKnown && f.Merge.IsBase:
			kept = append(kept, Kept{Project: f.Project, Reason: DefaultBranch})
		case merged:
			actions = append(actions, Action{Project: f.Project, Reason: Merged, Volumes: true})
		default:
			kept = append(kept, Kept{Project: f.Project, Reason: Unmerged})
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].Project < actions[j].Project })
	sort.Slice(kept, func(i, j int) bool { return kept[i].Project < kept[j].Project })
	return actions, kept
}
