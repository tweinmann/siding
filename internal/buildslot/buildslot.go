// Package buildslot limits how many siding processes build images at the same time: N lock
// files under ~/.siding/builds, held with flock. The kernel releases a lock when its process
// ends, so a killed up frees its slot at once, and there is no service.
package buildslot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Slot is a held build slot.
type Slot struct {
	f *os.File
}

// TryAcquire takes a free one of n slots under home without waiting; ok is false when all are
// held.
func TryAcquire(home string, n int) (s *Slot, ok bool, err error) {
	dir := filepath.Join(home, ".siding", "builds")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false, fmt.Errorf("build slots: %w", err)
	}
	for i := range max(n, 1) {
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("slot-%d.lock", i)), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return nil, false, fmt.Errorf("build slots: %w", err)
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &Slot{f: f}, true, nil
		}
		_ = f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, fmt.Errorf("build slots: %w", err)
		}
	}
	return nil, false, nil
}

// Release frees the slot.
func (s *Slot) Release() {
	if s != nil && s.f != nil {
		_ = s.f.Close() // closing the file drops the lock
		s.f = nil
	}
}
