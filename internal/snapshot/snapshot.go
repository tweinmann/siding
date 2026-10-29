// Package snapshot names, labels and groups snapshots: copies of a stack's named volumes, kept
// as Docker volumes. Everything here is a pure function; copying happens through a helper
// container (see internal/docker).
package snapshot

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
)

// Image is the helper image that copies volumes; the devcontainer's SIDING_BUSYBOX_IMAGE is the
// same.
const Image = "busybox:1.37.0"

// Labels of a snapshot volume.
const (
	LabelName    = "dev.siding.snapshot"
	LabelVolume  = "dev.siding.snapshot-volume"
	LabelCreated = "dev.siding.snapshot-created"
)

var validName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName fails with InvalidArguments unless name is lowercase letters, digits and single
// dashes, at most 40 characters.
func ValidName(name string) error {
	if len(name) > 40 || !validName.MatchString(name) {
		return fail.New(fail.InvalidArguments, "snapshot name %q: use lowercase letters, digits and dashes, at most 40 characters", name)
	}
	if name == ReservedName {
		return fail.New(fail.InvalidArguments, "the snapshot name %q is reserved: --seed %s starts a stack empty", name, name).
			WithHint("choose another name")
	}
	return nil
}

const (
	// DefaultName is the snapshot a first up seeds from without --seed (spec seeding).
	DefaultName = "default"
	// ReservedName is not a snapshot name: --seed none means no snapshot.
	ReservedName = "none"
)

// VolumeName is the volume holding one volume of a snapshot. The parts are separated by "_",
// which neither slugs nor snapshot names contain, so two snapshots never share a volume.
func VolumeName(repo, name, key string) string {
	return "siding-snap_" + stack.Slug(repo) + "_" + name + "_" + stack.Slug(key)
}

// Labels are the labels of a snapshot volume.
func Labels(repo, name, key, machine string, created time.Time) map[string]string {
	return map[string]string{
		LabelName:          name,
		LabelVolume:        key,
		LabelCreated:       created.UTC().Format(time.RFC3339),
		stack.LabelRepo:    repo,
		stack.LabelMachine: machine,
	}
}

// Snapshot is one snapshot with its volumes.
type Snapshot struct {
	Name    string
	Repo    string
	Created string
	// Volumes maps the stack's volume key to the snapshot volume.
	Volumes map[string]string
	// Size is the sum of the volumes' sizes, -1 when Docker does not know one of them.
	Size int64
}

// Group turns snapshot volumes into snapshots, sorted by repository and name. sizes may be nil.
func Group(vols []docker.Volume, sizes map[string]int64) []Snapshot {
	byKey := map[string]*Snapshot{}
	for _, v := range vols {
		name, repo := v.Labels[LabelName], v.Labels[stack.LabelRepo]
		if name == "" {
			continue
		}
		k := repo + "\x00" + name
		s, ok := byKey[k]
		if !ok {
			s = &Snapshot{Name: name, Repo: repo, Created: v.Labels[LabelCreated], Volumes: map[string]string{}}
			byKey[k] = s
		}
		s.Volumes[v.Labels[LabelVolume]] = v.Name
		switch size, known := sizes[v.Name]; {
		case sizes == nil:
		case !known || size < 0 || s.Size < 0:
			s.Size = -1
		default:
			s.Size += size
		}
	}
	out := make([]Snapshot, 0, len(byKey))
	for _, s := range byKey {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Find returns the snapshot name of repo, failing with SnapshotNotFound.
func Find(all []Snapshot, repo, name string) (Snapshot, error) {
	var names []string
	for _, s := range all {
		if s.Repo != repo {
			continue
		}
		if s.Name == name {
			return s, nil
		}
		names = append(names, s.Name)
	}
	e := fail.New(fail.SnapshotNotFound, "the repository %s has no snapshot %q", repo, name)
	if len(names) == 0 {
		return Snapshot{}, e.WithHint("save one with siding snapshot save <name> in a stack of %s", repo)
	}
	return Snapshot{}, e.WithHint("its snapshots are: %v", names)
}

// CopyHelper copies the volume from into the volume to.
func CopyHelper(from, to, machine string) docker.Helper {
	return docker.Helper{
		Image:  Image,
		Binds:  []string{from + ":/from:ro", to + ":/to"},
		Cmd:    []string{"sh", "-c", "cp -a /from/. /to/"},
		Labels: map[string]string{stack.LabelMachine: machine, "dev.siding.helper": "copy"},
	}
}

// ExistsError is the error of saving a snapshot whose name is taken.
func ExistsError(repo, name string) error {
	return fail.New(fail.SnapshotExists, "the repository %s already has a snapshot %q", repo, name).
		WithHint("choose another name, or replace it with --force")
}

// String is used in messages.
func (s Snapshot) String() string { return fmt.Sprintf("%s/%s", s.Repo, s.Name) }
