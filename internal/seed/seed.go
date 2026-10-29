// Package seed decides how a stack's first up fills it with data: which volumes come from a
// snapshot, and which services run a seed command. Everything here is a pure function.
package seed

import (
	"slices"
	"sort"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
)

// Labels on a service.
const (
	Label        = "dev.siding.seed"
	LabelCommand = "dev.siding.seed.command"
)

// Values of Label.
const (
	Volume = "volume"
	None   = "none"
	Script = "script"
)

// Run is a seed command to run once in a service.
type Run struct {
	Service string
	Command string
}

// Plan is what a first up seeds.
type Plan struct {
	// Volumes are the keys of the own named volumes that come from the snapshot.
	Volumes []string
	// Scripts run in compose order once their service is ready.
	Scripts []Run
}

// Make validates the seed labels of p and returns the plan of a first up. Volumes are only
// planned when there is a snapshot to seed from; scripts always run on a first up.
func Make(p *compose.Project, withSnapshot bool) (Plan, error) {
	var plan Plan
	notFromSnapshot := map[string]bool{}
	for _, name := range p.ServiceNames() {
		s := p.Services[name]
		switch mode := s.Labels[Label]; mode {
		case "", Volume:
		case None:
			for _, v := range s.Volumes {
				notFromSnapshot[v] = true
			}
		case Script:
			cmd := s.Labels[LabelCommand]
			if cmd == "" {
				return Plan{}, fail.New(fail.InvalidArguments, "service %s has %s: %s but no %s", name, Label, Script, LabelCommand).
					WithHint("add the label %s with the command to run once, e.g. \"npm run db:seed\"", LabelCommand)
			}
			plan.Scripts = append(plan.Scripts, Run{Service: name, Command: cmd})
			for _, v := range s.Volumes {
				notFromSnapshot[v] = true
			}
		default:
			return Plan{}, fail.New(fail.InvalidArguments, "service %s: %s must be %s, %s or %s, got %q", name, Label, Volume, None, Script, mode)
		}
	}
	if withSnapshot {
		for key, v := range p.Volumes {
			if !v.External && !notFromSnapshot[key] {
				plan.Volumes = append(plan.Volumes, key)
			}
		}
		sort.Strings(plan.Volumes)
	}
	return plan, nil
}

// FirstUp reports whether the project has never run: no containers and no volumes yet.
func FirstUp(containers, volumes int) bool { return containers == 0 && volumes == 0 }

// Uses reports whether the plan seeds anything.
func (p Plan) Uses() bool { return len(p.Volumes) > 0 || len(p.Scripts) > 0 }

// Has reports whether the plan seeds the volume key.
func (p Plan) Has(key string) bool { return slices.Contains(p.Volumes, key) }
