package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"mcp-pipedrive/pipedrive"
)

// scriptedTransport returns queued responses in order and records every
// request + body. Used by multi-request handlers (deal_fields.add_option,
// activities.batch_create).
type scriptedTransport struct {
	mu        sync.Mutex
	responses []string
	requests  []*http.Request
	bodies    [][]byte
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	s.requests = append(s.requests, req)
	s.bodies = append(s.bodies, body)
	resp := `{"success":true,"data":{}}`
	status := 200
	if len(s.responses) > 0 {
		resp = s.responses[0]
		s.responses = s.responses[1:]
	}
	if strings.HasPrefix(resp, "ERR:") {
		status = 400
		resp = strings.TrimPrefix(resp, "ERR:")
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(bytes.NewBufferString(resp)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func newScriptedCtx(t *testing.T, responses []string) (context.Context, *scriptedTransport) {
	t.Helper()
	transport := &scriptedTransport{responses: responses}
	client := &pipedrive.Client{
		BaseURL:  "https://test.pipedrive.com",
		HTTP:     &http.Client{Transport: transport},
		APIToken: "test-token",
		AuthMode: pipedrive.AuthModeToken,
	}
	ctx := pipedrive.WithConfig(context.Background(), pipedrive.Config{AllowWrite: true, AllowDelete: true})
	ctx = pipedrive.WithClient(ctx, client)
	return ctx, transport
}

func TestDealFieldsAddOption_MergesExistingAndAppendsNew(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":55,"field_type":"enum","options":[{"id":1,"label":"Existing"}]}}`,
		`{"success":true,"data":{"id":55,"field_type":"enum","options":[{"id":1,"label":"Existing"},{"id":2,"label":"New"}]}}`,
	})
	res, err := dealFieldsAddOption(ctx, DealFieldsAddOptionParams{FieldID: 55, Options: []string{"New", "Existing"}})
	if err != nil {
		t.Fatalf("dealFieldsAddOption: %v", err)
	}
	if len(transport.requests) != 2 {
		t.Fatalf("expected GET+PUT (2 requests), got %d", len(transport.requests))
	}
	if transport.requests[0].Method != "GET" || transport.requests[0].URL.Path != "/api/v1/dealFields/55" {
		t.Errorf("first request = %s %s, want GET /api/v1/dealFields/55", transport.requests[0].Method, transport.requests[0].URL.Path)
	}
	if transport.requests[1].Method != "PUT" || transport.requests[1].URL.Path != "/api/v1/dealFields/55" {
		t.Errorf("second request = %s %s, want PUT /api/v1/dealFields/55", transport.requests[1].Method, transport.requests[1].URL.Path)
	}
	var putBody struct {
		Options []map[string]any `json:"options"`
	}
	if err := json.Unmarshal(transport.bodies[1], &putBody); err != nil {
		t.Fatalf("unmarshal PUT body: %v", err)
	}
	if len(putBody.Options) != 2 {
		t.Fatalf("PUT options = %v, want existing+new (2)", putBody.Options)
	}
	if putBody.Options[0]["id"].(float64) != 1 || putBody.Options[0]["label"] != "Existing" {
		t.Errorf("existing option must keep its id: %v", putBody.Options[0])
	}
	if _, hasID := putBody.Options[1]["id"]; hasID || putBody.Options[1]["label"] != "New" {
		t.Errorf("new option must be label-only: %v", putBody.Options[1])
	}

	out, _ := json.Marshal(res)
	if !strings.Contains(string(out), `"added":["New"]`) || !strings.Contains(string(out), `"skipped":["Existing"]`) {
		t.Errorf("result should report added=[New] skipped=[Existing], got %s", out)
	}
}

func TestDealFieldsAddOption_RejectsNonEnumField(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":55,"field_type":"varchar","options":null}}`,
	})
	if _, err := dealFieldsAddOption(ctx, DealFieldsAddOptionParams{FieldID: 55, Options: []string{"X"}}); err == nil {
		t.Fatal("expected error for non-enum field")
	}
	if len(transport.requests) != 1 {
		t.Fatalf("must not PUT after rejecting field type; requests = %d", len(transport.requests))
	}
}

func TestDealFieldsAddOption_RejectsEmptyLabel(t *testing.T) {
	ctx, transport := newScriptedCtx(t, nil)
	if _, err := dealFieldsAddOption(ctx, DealFieldsAddOptionParams{FieldID: 55, Options: []string{"Valid", "  "}}); err == nil {
		t.Fatal("expected error for whitespace-only label")
	}
	if len(transport.requests) != 0 {
		t.Fatalf("must not make any request when a label is empty; requests = %d", len(transport.requests))
	}
}

func TestDealFieldsAddOption_AllDuplicatesSkipsPut(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":55,"field_type":"set","options":[{"id":1,"label":"A"}]}}`,
	})
	if _, err := dealFieldsAddOption(ctx, DealFieldsAddOptionParams{FieldID: 55, Options: []string{"A"}}); err != nil {
		t.Fatalf("dealFieldsAddOption: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("all-duplicate input must skip the PUT; requests = %d", len(transport.requests))
	}
}
