package snapshot

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
)

func code(err error) string {
	var e *fail.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestValidName(t *testing.T) {
	t.Parallel()
	for name, ok := range map[string]bool{"base": true, "big-2": true, "Base": false, "a--b": false, "-a": false, "a_b": false, "": false,
		"0123456789012345678901234567890123456789": true, "01234567890123456789012345678901234567890": false} {
		if err := ValidName(name); (err == nil) != ok || (!ok && code(err) != fail.InvalidArguments) {
			t.Errorf("ValidName(%q) = %v, want ok %v", name, err, ok)
		}
	}
}

func TestVolumeName(t *testing.T) {
	t.Parallel()
	if got := VolumeName("shop", "base", "pg_data"); got != "siding-snap_shop_base_pg-data" {
		t.Errorf("got %q", got)
	}
	// Dashes in repositories and names cannot make two snapshots share a volume.
	if VolumeName("a-b", "c", "d") == VolumeName("a", "b-c", "d") {
		t.Error("collision")
	}
}

func TestGroupAndFind(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	vol := func(repo, name, key string) docker.Volume {
		return docker.Volume{Name: VolumeName(repo, name, key), Labels: Labels(repo, name, key, "m", created)}
	}
	vols := []docker.Volume{vol("shop", "base", "data"), vol("shop", "base", "uploads"), vol("blog", "base", "data"), vol("shop", "big", "data"),
		{Name: "shop-main_data", Labels: map[string]string{"dev.siding.repo": "shop"}}} // a stack volume, not a snapshot
	sizes := map[string]int64{VolumeName("shop", "base", "data"): 100, VolumeName("shop", "base", "uploads"): 20, VolumeName("blog", "base", "data"): -1}
	all := Group(vols, sizes)
	var names []string
	for _, s := range all {
		names = append(names, s.String())
	}
	if !reflect.DeepEqual(names, []string{"blog/base", "shop/base", "shop/big"}) {
		t.Fatalf("snapshots %v", names)
	}
	if all[1].Size != 120 || all[0].Size != -1 || all[2].Size != -1 || all[1].Created != "2026-10-28T12:00:00Z" ||
		all[1].Volumes["uploads"] != "siding-snap_shop_base_uploads" {
		t.Errorf("shop/base %+v, blog/base %+v, shop/big %+v", all[1], all[0], all[2])
	}

	if s, err := Find(all, "shop", "big"); err != nil || s.Name != "big" {
		t.Errorf("Find big: %+v, %v", s, err)
	}
	if _, err := Find(all, "shop", "nope"); code(err) != fail.SnapshotNotFound {
		t.Errorf("Find nope: %v", err)
	}
	if code(ExistsError("shop", "base")) != fail.SnapshotExists {
		t.Error("ExistsError has the wrong code")
	}
}
