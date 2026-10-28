// Package stack derives everything about a stack that follows from its worktree and its
// compose file without asking Docker: names, labels, routes and the override siding writes.
// Everything here is a pure function.
package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Labels siding puts on what it creates. The conventions in CLAUDE.md list them.
const (
	LabelRepo     = "dev.siding.repo"
	LabelBranch   = "dev.siding.branch"
	LabelWorktree = "dev.siding.worktree"
	LabelMachine  = "dev.siding.machine"
	LabelAgent    = "dev.siding.agent"
	LabelGitDir   = "dev.siding.gitdir"
	LabelBudget   = "dev.siding.budget"
	LabelShared   = "dev.siding.shared"
	// LabelHostAlias, on siding-proxy, is what host.docker.internal resolves to in it.
	LabelHostAlias = "dev.siding.host-alias"
	LabelRoute     = "dev.siding.route"
	LabelPort      = "dev.siding.port"

	// LabelProject is Compose's own label naming a container's project.
	LabelProject = "com.docker.compose.project"
	// LabelService is Compose's own label naming a container's service.
	LabelService = "com.docker.compose.service"
)

// maxSlug is the longest slug; longer names are cut and get a hash suffix.
const maxSlug = 40

// Slug turns a name into lowercase letters, digits and single dashes, at most 40 characters.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	s := b.String()
	if s == "" || len(s) > maxSlug {
		sum := sha256.Sum256([]byte(name))
		suffix := hex.EncodeToString(sum[:])[:6]
		if s == "" {
			return "x-" + suffix
		}
		s = strings.TrimRight(s[:maxSlug-7], "-") + "-" + suffix
	}
	return s
}

// Identity names one stack.
type Identity struct {
	Repo     string // repository name, e.g. shop
	Branch   string // branch name, e.g. feature/login
	Worktree string // absolute path of the worktree
	Machine  string // stable machine id
	Agent    string // optional agent name
	// GitDir is the repository's common git directory, so gc can still ask git about the branch
	// when the worktree is gone.
	GitDir string
}

// RepoSlug is the slug of the repository name.
func (id Identity) RepoSlug() string { return Slug(id.Repo) }

// BranchSlug is the slug of the branch.
func (id Identity) BranchSlug() string { return Slug(id.Branch) }

// Project is the Compose project, <repo-slug>-<branch-slug>.
func (id Identity) Project() string { return id.RepoSlug() + "-" + id.BranchSlug() }

// Host is the stack's host, <repo-slug>.<branch-slug>.localhost.
func (id Identity) Host() string { return id.RepoSlug() + "." + id.BranchSlug() + ".localhost" }

// ServiceHost is the host of one service, <service>.<repo-slug>.<branch-slug>.localhost.
func (id Identity) ServiceHost(service string) string { return Slug(service) + "." + id.Host() }

// Labels are the labels of the stack's networks and volumes. They never change between two
// ups of one project: Compose recreates a network whose labels changed.
func (id Identity) Labels() map[string]string {
	return map[string]string{
		LabelRepo:     id.Repo,
		LabelBranch:   id.Branch,
		LabelWorktree: id.Worktree,
		LabelMachine:  id.Machine,
	}
}

// ContainerLabels are the labels of the stack's containers: Labels plus the git directory and
// the agent, if any. Those two stay off networks and volumes: they are new in v0.2 or change
// between ups, and Compose recreates a network whose labels changed.
func (id Identity) ContainerLabels() map[string]string {
	l := id.Labels()
	if id.GitDir != "" {
		l[LabelGitDir] = id.GitDir
	}
	if id.Agent != "" {
		l[LabelAgent] = id.Agent
	}
	return l
}

// FromLabels reads an Identity back from a container's labels.
func FromLabels(l map[string]string) Identity {
	return Identity{
		Repo:     l[LabelRepo],
		Branch:   l[LabelBranch],
		Worktree: l[LabelWorktree],
		Machine:  l[LabelMachine],
		Agent:    l[LabelAgent],
		GitDir:   l[LabelGitDir],
	}
}
