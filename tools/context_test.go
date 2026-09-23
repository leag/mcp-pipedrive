package tools

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
)

// pathsTransport is a thread-safe transport that records every request path.
// contextGet fans out concurrent goroutines, so captureTransport (last-only,
// unsynchronized) would race.
type pathsTransport struct {
	mu    sync.Mutex
	paths []string
}

func (p *pathsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.paths = append(p.paths, req.URL.Path)
	p.mu.Unlock()
	body := bytes.NewBufferString(`{"success":true,"data":[]}`)
	return &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(body),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func (p *pathsTransport) has(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, v := range p.paths {
		if v == path {
			return true
		}
	}
	return false
}

func TestContextGet_FetchesActivityTypes(t *testing.T) {
	transport := &pathsTransport{}
	ctx, _ := newTestCtx(t)
	// Swap in the concurrent-safe transport.
	client := clientFromTestCtx(t, ctx)
	client.HTTP.Transport = transport

	if _, err := contextGet(ctx, ContextGetParams{}); err != nil {
		t.Fatalf("contextGet: %v", err)
	}
	for _, want := range []string{
		"/api/v1/users",
		"/api/v2/pipelines",
		"/api/v2/stages",
		"/api/v1/dealFields",
		"/api/v1/activityTypes",
	} {
		if !transport.has(want) {
			t.Errorf("contextGet did not call %s (called: %v)", want, transport.paths)
		}
	}
}
