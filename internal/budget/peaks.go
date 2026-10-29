package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tweinmann/siding/internal/compose"
)

// PeakMargin is added to a learned peak: one-shot stats see the use now, not the maximum.
const PeakMargin = 1.25

// Peaks is the highest memory use siding has seen per "<repository>/<service>". It is kept in
// ~/.siding/peaks.json, a losable cache.
type Peaks map[string]int64

// PeakKey is the key of a service of a repository.
func PeakKey(repo, service string) string { return repo + "/" + service }

func peaksPath(home string) string { return filepath.Join(home, ".siding", "peaks.json") }

// LoadPeaks reads the cache; a missing or broken file is an empty cache.
func LoadPeaks(home string) Peaks {
	data, err := os.ReadFile(peaksPath(home))
	if err != nil {
		return Peaks{}
	}
	var p Peaks
	if err := json.Unmarshal(data, &p); err != nil || p == nil {
		return Peaks{}
	}
	return p
}

// Learn records samples, keeping the highest value per key, and writes the cache atomically. A
// sample another siding process wrote meanwhile may be lost; it is a cache.
func Learn(home string, samples Peaks) error {
	if len(samples) == 0 {
		return nil
	}
	p := LoadPeaks(home)
	changed := false
	for k, v := range samples {
		if v > p[k] {
			p[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := peaksPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write learned peaks: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".peaks-*.json")
	if err != nil {
		return fmt.Errorf("write learned peaks: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write learned peaks: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// EstimateLearned is the memory a project of repo will need: per service and replica its
// learned peak plus PeakMargin, capped at its memory limit; else the limit; else
// ServiceDefault.
func EstimateLearned(p *compose.Project, repo string, peaks Peaks) int64 {
	var sum int64
	for name, s := range p.Services {
		per := s.MemoryLimit
		if peak, ok := peaks[PeakKey(repo, name)]; ok && peak > 0 {
			per = int64(float64(peak) * PeakMargin)
			if s.MemoryLimit > 0 && per > s.MemoryLimit {
				per = s.MemoryLimit
			}
		}
		if per <= 0 {
			per = ServiceDefault
		}
		replicas := s.Replicas
		if replicas < 1 {
			replicas = 1
		}
		sum += per * int64(replicas)
	}
	return sum
}
