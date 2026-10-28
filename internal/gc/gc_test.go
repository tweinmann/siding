package gc

import (
	"reflect"
	"testing"

	"github.com/tweinmann/siding/internal/git"
)

func TestPlan(t *testing.T) {
	t.Parallel()
	base := "refs/heads/main"
	mergedState := git.MergeState{Base: base, Exists: true, Ancestor: true}
	tests := []struct {
		name   string
		facts  Facts
		action *Action
		kept   *Kept
	}{
		{
			name:   "merged and clean",
			facts:  Facts{Project: "p", WorktreeExists: true, Merge: mergedState, MergeKnown: true},
			action: &Action{Project: "p", Reason: Merged, Volumes: true},
		},
		{
			name:   "squash-merged: upstream gone",
			facts:  Facts{Project: "p", WorktreeExists: true, Merge: git.MergeState{Base: base, Exists: true, UpstreamGone: true}, MergeKnown: true},
			action: &Action{Project: "p", Reason: Merged, Volumes: true},
		},
		{
			name:  "merged but dirty",
			facts: Facts{Project: "p", WorktreeExists: true, Dirty: true, Merge: mergedState, MergeKnown: true},
			kept:  &Kept{Project: "p", Reason: Dirty},
		},
		{
			name:  "unmerged",
			facts: Facts{Project: "p", WorktreeExists: true, Merge: git.MergeState{Base: base, Exists: true}, MergeKnown: true},
			kept:  &Kept{Project: "p", Reason: Unmerged},
		},
		{
			name:  "a fresh branch without own commits",
			facts: Facts{Project: "p", WorktreeExists: true, Merge: git.MergeState{Base: base, Exists: true, Ancestor: true, AtBase: true}, MergeKnown: true},
			kept:  &Kept{Project: "p", Reason: Unmerged},
		},
		{
			name:  "the default branch",
			facts: Facts{Project: "p", WorktreeExists: true, Merge: git.MergeState{Base: base, Exists: true, Ancestor: true, AtBase: true, IsBase: true}, MergeKnown: true},
			kept:  &Kept{Project: "p", Reason: DefaultBranch},
		},
		{
			name:   "worktree gone, branch merged",
			facts:  Facts{Project: "p", Merge: mergedState, MergeKnown: true},
			action: &Action{Project: "p", Reason: WorktreeGone, Volumes: true},
		},
		{
			name:   "worktree gone, branch unmerged",
			facts:  Facts{Project: "p", Merge: git.MergeState{Base: base, Exists: true}, MergeKnown: true},
			action: &Action{Project: "p", Reason: WorktreeGone},
		},
		{
			name:   "worktree gone, a stack of siding v0.1 without the gitdir label",
			facts:  Facts{Project: "p"},
			action: &Action{Project: "p", Reason: WorktreeGone},
		},
		{
			name:  "merge state unknown although the worktree exists",
			facts: Facts{Project: "p", WorktreeExists: true},
			kept:  &Kept{Project: "p", Reason: Unmerged},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.facts.Machine = "m"
			actions, kept := Plan([]Facts{tt.facts}, "m")
			var gotAction *Action
			var gotKept *Kept
			if len(actions) == 1 {
				gotAction = &actions[0]
			}
			if len(kept) == 1 {
				gotKept = &kept[0]
			}
			if len(actions)+len(kept) != 1 || !reflect.DeepEqual(gotAction, tt.action) || !reflect.DeepEqual(gotKept, tt.kept) {
				t.Errorf("actions %+v, kept %+v; want %+v, %+v", actions, kept, tt.action, tt.kept)
			}
		})
	}
}

func TestPlanSorted(t *testing.T) {
	t.Parallel()
	actions, kept := Plan([]Facts{{Project: "b", Machine: "m"}, {Project: "a", Machine: "m"}, {Project: "d", Machine: "m", WorktreeExists: true}, {Project: "c", Machine: "m", WorktreeExists: true}}, "m")
	if actions[0].Project != "a" || kept[0].Project != "c" {
		t.Errorf("not sorted: %+v %+v", actions, kept)
	}
}

func TestPlanOtherMachines(t *testing.T) {
	t.Parallel()
	actions, kept := Plan([]Facts{{Project: "remote", Machine: "other"}}, "m")
	if len(actions)+len(kept) != 0 {
		t.Errorf("a stack of another machine: %+v %+v", actions, kept)
	}
}
