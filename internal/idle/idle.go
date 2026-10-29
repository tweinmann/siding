// Package idle tells when a stack was last requested through the proxy, from the proxy's access
// log: one JSON line per request with only the router and the time (phase 4 design D3, D4).
package idle

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/tweinmann/siding/internal/proxy"
)

// Recording reports whether a proxy with this command records its requests; a proxy of siding
// v0.3 does not.
func Recording(cmd []string) bool {
	return slices.Contains(cmd, proxy.AccessLogFlag)
}

// line is what Traefik's JSON access log keeps of a request.
type line struct {
	RouterName string
	StartUTC   time.Time
}

// LastRequests returns the latest request per project in the proxy's log lines. A router belongs
// to the longest project name it starts with, followed by "-": router names are
// <project>-<service>, so shop-main-x-web belongs to shop-main-x, not to shop-main. Lines that
// are not access log lines, such as Traefik's own, are skipped.
func LastRequests(lines []string, projects []string) map[string]time.Time {
	byLength := slices.Clone(projects)
	slices.SortFunc(byLength, func(a, b string) int { return len(b) - len(a) })
	last := map[string]time.Time{}
	for _, l := range lines {
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var req line
		if err := json.Unmarshal([]byte(l), &req); err != nil || req.RouterName == "" || req.StartUTC.IsZero() {
			continue
		}
		router, _, _ := strings.Cut(req.RouterName, "@")
		for _, p := range byLength {
			if strings.HasPrefix(router, p+"-") {
				if req.StartUTC.After(last[p]) {
					last[p] = req.StartUTC
				}
				break
			}
		}
	}
	return last
}

// Since is the time a stack counts as last requested: its latest request, or the later start of
// the stack or the proxy when that is later, because the proxy's record begins when it starts
// and a new stack has not had its chance yet (spec idle-stop).
func Since(request, stackStart, proxyStart time.Time) time.Time {
	latest := request
	for _, t := range []time.Time{stackStart, proxyStart} {
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}
