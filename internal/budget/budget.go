// Package budget decides whether another stack fits into the memory the machine has for siding.
// Everything here is a pure function; the numbers come from Docker and the compose config.
package budget

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tweinmann/siding/internal/compose"
)

const (
	// ServiceDefault is the estimate of a service without a memory limit. It is a guess, until
	// siding learns real peaks (v0.3).
	ServiceDefault int64 = 256 << 20
	// DefaultShare is the part of Docker's memory that is the budget when none is set.
	DefaultShare = 0.75
)

// Default is the budget without a label: 75 % of the memory the Docker daemon reports.
func Default(dockerMemory int64) int64 {
	return int64(float64(dockerMemory) * DefaultShare)
}

// ParseSize reads a size as bytes or with the binary suffix k, m or g: 512m, 12g, 1073741824.
func ParseSize(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	shift := 0
	switch {
	case strings.HasSuffix(s, "k"):
		shift = 10
	case strings.HasSuffix(s, "m"):
		shift = 20
	case strings.HasSuffix(s, "g"):
		shift = 30
	}
	if shift > 0 {
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > (1<<62)>>shift {
		return 0, fmt.Errorf("not a size: use bytes or a number with k, m or g, such as 512m or 12g")
	}
	return n << shift, nil
}

// Estimate is the memory a project will need: per service and replica its memory limit, or
// ServiceDefault without one.
func Estimate(p *compose.Project) int64 {
	var sum int64
	for _, s := range p.Services {
		per := s.MemoryLimit
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

// Check is the outcome of fitting a stack into the budget.
type Check struct {
	Budget   int64
	Used     int64 // memory of every other stack
	Estimate int64 // of this stack
}

// Fits reports whether the stack fits.
func (c Check) Fits() bool { return c.Used+c.Estimate <= c.Budget }
