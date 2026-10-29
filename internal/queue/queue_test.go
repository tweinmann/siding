package queue

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLine(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dead := map[int]bool{}
	l := Open(home, func(pid int) bool { return !dead[pid] })
	t0 := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	if line, err := l.List(); err != nil || len(line) != 0 {
		t.Fatalf("empty: %v, %v", line, err)
	}
	leaveA, err := l.Enter(Entry{Project: "a", PID: 101, Estimate: 1, Since: t0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Enter(Entry{Project: "b", PID: 102, Estimate: 2, Since: t0.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Enter(Entry{Project: "c", PID: 103, Estimate: 3, Since: t0.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	line, _ := l.List()
	if len(line) != 3 || line[0].Project != "a" || Place(line, 103) != 3 || Place(line, 999) != 0 {
		t.Fatalf("line %+v", line)
	}

	// a leaves; b is killed: c moves to the front, and b's file is cleaned up.
	leaveA()
	dead[102] = true
	line, _ = l.List()
	if len(line) != 1 || Place(line, 103) != 1 {
		t.Fatalf("after leaving and a kill: %+v", line)
	}
	files, _ := os.ReadDir(filepath.Join(home, ".siding", "queue"))
	if len(files) != 1 {
		t.Errorf("files left: %v", files)
	}
}

func TestBrokenEntry(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	l := Open(home, func(int) bool { return true })
	dir := filepath.Join(home, ".siding", "queue")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-1.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if line, err := l.List(); err != nil || len(line) != 0 {
		t.Errorf("a broken entry: %v, %v", line, err)
	}
}
