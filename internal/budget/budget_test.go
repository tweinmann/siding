package budget

import (
	"testing"

	"github.com/tweinmann/siding/internal/compose"
)

const mib = 1 << 20

func TestParseSize(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in   string
		want int64
	}{
		{"512m", 512 * mib}, {"12g", 12 << 30}, {"12G", 12 << 30}, {"64k", 64 << 10}, {"1073741824", 1 << 30}, {" 2g ", 2 << 30},
		{"", 0}, {"0", 0}, {"-1g", 0}, {"12gb", 0}, {"lots", 0}, {"99999999999g", 0},
	} {
		got, err := ParseSize(tt.in)
		if (tt.want == 0) != (err != nil) || got != tt.want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
}

func TestEstimate(t *testing.T) {
	t.Parallel()
	p := &compose.Project{Services: map[string]compose.Service{
		"db":     {MemoryLimit: 1024 * mib, Replicas: 1},
		"web":    {Replicas: 1},
		"api":    {MemoryLimit: 512 * mib, Replicas: 1},
		"worker": {MemoryLimit: 100 * mib, Replicas: 3},
		"old":    {}, // a project parsed before replicas existed counts once
	}}
	want := int64(1024+256+512+300+256) * mib
	if got := Estimate(p); got != want {
		t.Errorf("Estimate = %d MiB, want %d MiB", got/mib, want/mib)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	if !(Check{Budget: 4096 * mib, Used: 3000 * mib, Estimate: 1096 * mib}).Fits() {
		t.Error("exactly the budget must fit")
	}
	if (Check{Budget: 4096 * mib, Used: 3584 * mib, Estimate: 768 * mib}).Fits() {
		t.Error("over the budget must not fit")
	}
	if got := Default(16 << 30); got != 12<<30 {
		t.Errorf("Default(16 GiB) = %d", got)
	}
}
