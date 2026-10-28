package stack

import (
	"bufio"
	"slices"
	"strconv"
	"strings"
)

// minSecret is the shortest value siding redacts; shorter ones would wreck every line.
const minSecret = 4

// Redactor replaces known secret values in log lines with ***.
type Redactor struct {
	values []string // longest first
}

// NewRedactor redacts the given values except short ones, numbers and booleans.
func NewRedactor(values ...string) *Redactor {
	seen := map[string]bool{}
	r := &Redactor{}
	for _, v := range values {
		if len(v) < minSecret || seen[v] || harmless(v) {
			continue
		}
		seen[v] = true
		r.values = append(r.values, v)
	}
	slices.SortFunc(r.values, func(a, b string) int { return len(b) - len(a) })
	return r
}

func harmless(v string) bool {
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off":
		return true
	}
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}

// Line returns line with every known value replaced by ***.
func (r *Redactor) Line(line string) string {
	for _, v := range r.values {
		line = strings.ReplaceAll(line, v, "***")
	}
	return line
}

// Lines redacts every line.
func (r *Redactor) Lines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = r.Line(l)
	}
	return out
}

// DotEnvValues returns the values a .env file defines, read with Compose's rules: KEY=VALUE
// per line, an optional "export ", comments starting with #, single or double quotes.
func DotEnvValues(content string) []string {
	var values []string
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		_, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch {
		case len(value) >= 2 && (value[0] == '"' || value[0] == '\''):
			if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
				value = value[1 : end+1]
			}
		default:
			if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
