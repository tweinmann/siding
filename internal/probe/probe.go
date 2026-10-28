// Package probe asks the proxy whether a routed service answers, without DNS: the request goes
// to 127.0.0.1 on the proxy's port with the stack's host in the Host header.
package probe

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Response is what the proxy answered.
type Response struct {
	Status int
	// Service is the value of the X-Siding-Service header, set only on the service's own
	// responses.
	Service string
}

// Answered reports whether the response came from service (<project>/<service>) itself and
// not from the proxy: it carries the service header and is not a gateway error.
func (r Response) Answered(service string) bool {
	switch r.Status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false
	}
	return r.Service == service
}

// HTTP probes over plain HTTP.
type HTTP struct {
	// Header is the response header naming the service.
	Header string
}

// Get sends GET path with Host: host to addr (host:port) and follows no redirect.
func (p HTTP) Get(ctx context.Context, addr, host, path string) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return Response{}, err
	}
	req.Host = host
	c := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return Response{Status: resp.StatusCode, Service: resp.Header.Get(p.Header)}, nil
}
