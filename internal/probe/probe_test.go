package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "shop.main.localhost" {
			http.Error(w, "404 page not found", http.StatusNotFound) // like Traefik without a router
			return
		}
		switch r.URL.Path {
		case "/":
			w.Header().Set("X-Siding-Service", "shop-main/web")
			http.Redirect(w, r, "/login", http.StatusFound)
		case "/api":
			w.Header().Set("X-Siding-Service", "shop-main/api")
			w.WriteHeader(http.StatusNotFound) // the app's own 404
		default:
			w.Header().Set("X-Siding-Service", "shop-main/gone")
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	p := HTTP{Header: "X-Siding-Service"}
	tests := []struct {
		name, host, path, service string
		answered                  bool
	}{
		{name: "redirect counts", host: "shop.main.localhost", path: "/", service: "shop-main/web", answered: true},
		{name: "the app's 404 counts", host: "shop.main.localhost", path: "/api", service: "shop-main/api", answered: true},
		{name: "the proxy's 404 does not", host: "other.localhost", path: "/", service: "shop-main/web"},
		{name: "a gateway error does not", host: "shop.main.localhost", path: "/x", service: "shop-main/gone"},
		{name: "another service's answer does not", host: "shop.main.localhost", path: "/api", service: "shop-main/web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := p.Get(context.Background(), addr, tt.host, tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if got := resp.Answered(tt.service); got != tt.answered {
				t.Errorf("response %+v: answered %v, want %v", resp, got, tt.answered)
			}
		})
	}
}
