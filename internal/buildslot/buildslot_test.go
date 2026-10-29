package buildslot

import "testing"

// flock locks belong to open files, so two acquisitions in one process exclude each other like
// two processes would.
func TestSlots(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	a, ok, err := TryAcquire(home, 2)
	if err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	b, ok, err := TryAcquire(home, 2)
	if err != nil || !ok {
		t.Fatalf("second: %v %v", ok, err)
	}
	if _, ok, err := TryAcquire(home, 2); err != nil || ok {
		t.Fatalf("third of two: %v %v", ok, err)
	}
	a.Release()
	c, ok, err := TryAcquire(home, 2)
	if err != nil || !ok {
		t.Fatalf("after a release: %v %v", ok, err)
	}
	b.Release()
	c.Release()
	c.Release() // twice is fine
	if s, ok, _ := TryAcquire(home, 0); !ok {
		t.Error("n below 1 still has one slot")
	} else {
		s.Release()
	}
}
