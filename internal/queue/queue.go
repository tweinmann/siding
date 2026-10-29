// Package queue is the line of up --wait processes waiting for memory. It has no service: each
// waiting process keeps one file under ~/.siding/queue, and every process reads the directory to
// learn its place. The order is the order of arrival; a process that ended leaves the line, also
// when it was killed, because a file whose process is gone counts as gone.
package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry is one waiting up.
type Entry struct {
	Project  string    `json:"project"`
	PID      int       `json:"pid"`
	Estimate int64     `json:"estimate"`
	Since    time.Time `json:"since"`
	file     string
}

// Line is the queue directory of one machine.
type Line struct {
	dir   string
	alive func(pid int) bool
}

// Open returns the line under home; alive tells whether a process still runs.
func Open(home string, alive func(pid int) bool) Line {
	return Line{dir: filepath.Join(home, ".siding", "queue"), alive: alive}
}

// Enter puts a process at the end of the line and returns the function that takes it out.
func (l Line) Enter(e Entry) (leave func(), err error) {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return nil, fmt.Errorf("enter the queue: %w", err)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	// The name sorts by arrival; the pid keeps two arrivals in the same nanosecond apart.
	name := fmt.Sprintf("%020d-%d.json", e.Since.UnixNano(), e.PID)
	path := filepath.Join(l.dir, name)
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return nil, fmt.Errorf("enter the queue: %w", err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return nil, fmt.Errorf("enter the queue: %w", err)
	}
	return func() { _ = os.Remove(path) }, nil
}

// List returns the line in order of arrival. It removes the files of processes that are gone.
func (l Line) List() ([]Entry, error) {
	files, err := os.ReadDir(l.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the queue: %w", err)
	}
	var line []Entry
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		path := filepath.Join(l.dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // left meanwhile
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil || !l.alive(e.PID) {
			_ = os.Remove(path)
			continue
		}
		e.file = f.Name()
		line = append(line, e)
	}
	sort.Slice(line, func(i, j int) bool { return line[i].file < line[j].file })
	return line, nil
}

// Place is the 1-based place of the process pid in line, or 0 when it is not in it.
func Place(line []Entry, pid int) int {
	for i, e := range line {
		if e.PID == pid {
			return i + 1
		}
	}
	return 0
}
