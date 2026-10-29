package budget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tweinmann/siding/internal/compose"
)

func TestEstimateLearned(t *testing.T) {
	t.Parallel()
	p := &compose.Project{Services: map[string]compose.Service{
		"db":  {MemoryLimit: 1024 * mib, Replicas: 1},
		"web": {Replicas: 2},
		"api": {MemoryLimit: 512 * mib, Replicas: 1},
	}}
	tests := []struct {
		name  string
		peaks Peaks
		want  int64
	}{
		{name: "nothing learned", want: (1024 + 2*256 + 512) * mib},
		{name: "a learned peak plus a quarter", peaks: Peaks{"shop/web": 400 * mib}, want: (1024 + 2*500 + 512) * mib},
		{name: "capped at the limit", peaks: Peaks{"shop/db": 900 * mib}, want: (1024 + 2*256 + 512) * mib},
		{name: "below the limit", peaks: Peaks{"shop/api": 200 * mib}, want: (1024 + 2*256 + 250) * mib},
		{name: "another repository's peak does not count", peaks: Peaks{"blog/web": 400 * mib}, want: (1024 + 2*256 + 512) * mib},
	}
	for _, tt := range tests {
		if got := EstimateLearned(p, "shop", tt.peaks); got != tt.want {
			t.Errorf("%s: %d MiB, want %d MiB", tt.name, got/mib, tt.want/mib)
		}
	}
}

func TestLearn(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if got := LoadPeaks(home); len(got) != 0 {
		t.Errorf("no file: %v", got)
	}
	if err := Learn(home, Peaks{"shop/web": 400 * mib}); err != nil {
		t.Fatal(err)
	}
	if err := Learn(home, Peaks{"shop/web": 300 * mib, "shop/db": 100 * mib}); err != nil {
		t.Fatal(err)
	}
	got := LoadPeaks(home)
	if got["shop/web"] != 400*mib || got["shop/db"] != 100*mib {
		t.Errorf("peaks %v", got)
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".siding"))
	if len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}

	// A broken cache is an empty one, and learning starts anew.
	if err := os.WriteFile(filepath.Join(home, ".siding", "peaks.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadPeaks(home); len(got) != 0 {
		t.Errorf("broken file: %v", got)
	}
	if err := Learn(home, Peaks{"shop/web": 10 * mib}); err != nil || LoadPeaks(home)["shop/web"] != 10*mib {
		t.Errorf("after a broken file: %v, %v", err, LoadPeaks(home))
	}
}
