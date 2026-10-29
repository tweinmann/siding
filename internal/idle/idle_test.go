package idle

import (
	"testing"
	"time"
)

func TestRecording(t *testing.T) {
	t.Parallel()
	if !Recording([]string{"--ping=true", "--accesslog=true"}) || Recording([]string{"--ping=true"}) {
		t.Error("only a proxy with --accesslog=true records its requests")
	}
}

func TestLastRequests(t *testing.T) {
	t.Parallel()
	at := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	lines := []string{
		// Traefik's own log is not JSON.
		`2026-10-29T18:27:13Z INF Loading plugins...`,
		`{"RouterName":"shop-main-web@docker","StartUTC":"2026-10-29T18:28:38.385704594Z","level":"info","msg":"","time":"2026-10-29T18:28:38Z"}`,
		`{"RouterName":"shop-main-web-route@docker","StartUTC":"2026-10-29T18:30:00Z","level":"info"}`,
		// An older request after a newer one: the latest counts.
		`{"RouterName":"shop-main-api@docker","StartUTC":"2026-10-29T18:29:00Z"}`,
		// shop-main-x is a project of its own, not a service x of shop-main.
		`{"RouterName":"shop-main-x-web@docker","StartUTC":"2026-10-29T18:31:00Z"}`,
		// The overview and unknown projects belong to no stack.
		`{"RouterName":"siding-ui@file","StartUTC":"2026-10-29T18:32:00Z"}`,
		`{"RouterName":"other-main-web@docker","StartUTC":"2026-10-29T18:33:00Z"}`,
		// Broken lines and lines without a router or a time.
		`{"RouterName":"shop-fix-web@docker","StartUTC":`,
		`{"StartUTC":"2026-10-29T18:34:00Z"}`,
		`{"RouterName":"shop-fix-web@docker"}`,
	}
	got := LastRequests(lines, []string{"shop-main", "shop-main-x", "shop-fix"})
	want := map[string]time.Time{"shop-main": at("2026-10-29T18:30:00Z"), "shop-main-x": at("2026-10-29T18:31:00Z")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for p, w := range want {
		if !got[p].Equal(w) {
			t.Errorf("%s: %v, want %v", p, got[p], w)
		}
	}
	if len(LastRequests(nil, []string{"shop-main"})) != 0 {
		t.Error("an empty record knows no request")
	}
}

func TestSince(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name                   string
		request, stack, proxyT time.Time
		want                   time.Time
	}{
		{name: "requested after both started", request: base.Add(-2 * time.Minute), stack: base.Add(-time.Hour), proxyT: base.Add(-2 * time.Hour), want: base.Add(-2 * time.Minute)},
		{name: "never requested", stack: base.Add(-20 * time.Minute), proxyT: base.Add(-time.Hour), want: base.Add(-20 * time.Minute)},
		{name: "the proxy was recreated", stack: base.Add(-time.Hour), proxyT: base.Add(-time.Minute), want: base.Add(-time.Minute)},
		{name: "started again after its last request", request: base.Add(-time.Hour), stack: base.Add(-3 * time.Minute), proxyT: base.Add(-2 * time.Hour), want: base.Add(-3 * time.Minute)},
	} {
		if got := Since(tt.request, tt.stack, tt.proxyT); !got.Equal(tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}
