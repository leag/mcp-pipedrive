package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"mcp-pipedrive/pipedrive"
)

// captureTransport records the most recent outgoing request (and its body)
// and returns a canned empty-list response. Lets us assert which query
// parameters and body fields a handler actually sends upstream without
// hitting Pipedrive.
type captureTransport struct {
	last     *http.Request
	lastBody []byte
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.last = req
	c.lastBody = nil
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		c.lastBody = b
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	body := bytes.NewBufferString(`{"success":true,"data":[]}`)
	return &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(body),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func newTestCtx(t *testing.T) (context.Context, *captureTransport) {
	t.Helper()
	transport := &captureTransport{}
	client := &pipedrive.Client{
		BaseURL:  "https://test.pipedrive.com",
		HTTP:     &http.Client{Transport: transport},
		APIToken: "test-token",
		AuthMode: pipedrive.AuthModeToken,
		// Cache nil → bypasses caching path entirely.
	}
	ctx := pipedrive.WithConfig(context.Background(), pipedrive.Config{})
	ctx = pipedrive.WithClient(ctx, client)
	return ctx, transport
}

// newWriteTestCtx is newTestCtx with write+delete flags enabled, for
// exercising gated handlers end-to-end.
func newWriteTestCtx(t *testing.T) (context.Context, *captureTransport) {
	t.Helper()
	ctx, transport := newTestCtx(t)
	ctx = pipedrive.WithConfig(ctx, pipedrive.Config{AllowWrite: true, AllowDelete: true})
	return ctx, transport
}

// mustBody unmarshals the JSON body of the last captured request.
func mustBody(t *testing.T, transport *captureTransport) map[string]any {
	t.Helper()
	if transport.lastBody == nil {
		t.Fatal("handler did not send a request body")
	}
	var body map[string]any
	if err := json.Unmarshal(transport.lastBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v (raw: %s)", err, transport.lastBody)
	}
	return body
}

func mustQuery(t *testing.T, transport *captureTransport) url.Values {
	t.Helper()
	if transport.last == nil {
		t.Fatal("handler did not issue an HTTP request")
	}
	return transport.last.URL.Query()
}

// assertRequest pins the upstream method + path so a refactor that
// accidentally retargets a list handler (e.g. /api/v2/deals → /api/v2/deal)
// is caught instead of silently shipping.
func assertRequest(t *testing.T, transport *captureTransport, wantMethod, wantPath string) {
	t.Helper()
	if transport.last == nil {
		t.Fatal("handler did not issue an HTTP request")
	}
	if got := transport.last.Method; got != wantMethod {
		t.Errorf("HTTP method = %q, want %q", got, wantMethod)
	}
	if got := transport.last.URL.Path; got != wantPath {
		t.Errorf("URL path = %q, want %q", got, wantPath)
	}
}

func assertHas(t *testing.T, q url.Values, key, want string) {
	t.Helper()
	if got := q.Get(key); got != want {
		t.Errorf("query %s = %q, want %q (full query: %v)", key, got, want, q)
	}
}

func assertAbsent(t *testing.T, q url.Values, key string) {
	t.Helper()
	if got, ok := q[key]; ok {
		t.Errorf("query %s should be absent but was %v (full query: %v)", key, got, q)
	}
}

func TestDealsList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{
		FilterID:     42,
		UpdatedSince: "2026-05-01T00:00:00Z",
		UpdatedUntil: "2026-05-31T00:00:00Z",
	}); err != nil {
		t.Fatalf("dealsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v2/deals")
	assertHas(t, q, "filter_id", "42")
	assertHas(t, q, "updated_since", "2026-05-01T00:00:00Z")
	assertHas(t, q, "updated_until", "2026-05-31T00:00:00Z")

	ctx, transport = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{}); err != nil {
		t.Fatalf("dealsList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "updated_since")
	assertAbsent(t, q, "updated_until")
}

func TestPersonsList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := personsList(ctx, PersonsListParams{
		FilterID:     7,
		UpdatedSince: "2026-01-01T00:00:00Z",
		UpdatedUntil: "2026-12-31T00:00:00Z",
	}); err != nil {
		t.Fatalf("personsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v2/persons")
	assertHas(t, q, "filter_id", "7")
	assertHas(t, q, "updated_since", "2026-01-01T00:00:00Z")
	assertHas(t, q, "updated_until", "2026-12-31T00:00:00Z")

	ctx, transport = newTestCtx(t)
	if _, err := personsList(ctx, PersonsListParams{}); err != nil {
		t.Fatalf("personsList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "updated_since")
	assertAbsent(t, q, "updated_until")
}

func TestOrganizationsList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := organizationsList(ctx, OrganizationsListParams{
		FilterID:     99,
		UpdatedSince: "2026-03-01T00:00:00Z",
		UpdatedUntil: "2026-04-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("organizationsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v2/organizations")
	assertHas(t, q, "filter_id", "99")
	assertHas(t, q, "updated_since", "2026-03-01T00:00:00Z")
	assertHas(t, q, "updated_until", "2026-04-01T00:00:00Z")

	ctx, transport = newTestCtx(t)
	if _, err := organizationsList(ctx, OrganizationsListParams{}); err != nil {
		t.Fatalf("organizationsList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "updated_since")
	assertAbsent(t, q, "updated_until")
}

func TestActivitiesList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := activitiesList(ctx, ActivitiesListParams{
		FilterID:     11,
		DueDate:      "2026-05-22",
		UpdatedSince: "2026-05-01T00:00:00Z",
		UpdatedUntil: "2026-05-31T00:00:00Z",
	}); err != nil {
		t.Fatalf("activitiesList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v2/activities")
	assertHas(t, q, "filter_id", "11")
	assertHas(t, q, "due_date", "2026-05-22")
	assertHas(t, q, "updated_since", "2026-05-01T00:00:00Z")
	assertHas(t, q, "updated_until", "2026-05-31T00:00:00Z")

	ctx, transport = newTestCtx(t)
	if _, err := activitiesList(ctx, ActivitiesListParams{}); err != nil {
		t.Fatalf("activitiesList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "due_date")
	assertAbsent(t, q, "updated_since")
	assertAbsent(t, q, "updated_until")
}

func TestProductsList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := productsList(ctx, ProductsListParams{
		FilterID:     3,
		UpdatedSince: "2026-02-01T00:00:00Z",
		UpdatedUntil: "2026-03-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("productsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v2/products")
	assertHas(t, q, "filter_id", "3")
	assertHas(t, q, "updated_since", "2026-02-01T00:00:00Z")
	assertHas(t, q, "updated_until", "2026-03-01T00:00:00Z")

	ctx, transport = newTestCtx(t)
	if _, err := productsList(ctx, ProductsListParams{}); err != nil {
		t.Fatalf("productsList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "updated_since")
	assertAbsent(t, q, "updated_until")
}

func TestLeadsList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := leadsList(ctx, LeadsListParams{FilterID: 55}); err != nil {
		t.Fatalf("leadsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v1/leads")
	assertHas(t, q, "filter_id", "55")

	ctx, transport = newTestCtx(t)
	if _, err := leadsList(ctx, LeadsListParams{}); err != nil {
		t.Fatalf("leadsList zero: %v", err)
	}
	assertAbsent(t, mustQuery(t, transport), "filter_id")
}

func TestNotesList_QueryPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := notesList(ctx, NotesListParams{
		FilterID:  17,
		StartDate: "2026-05-01",
		EndDate:   "2026-05-31",
	}); err != nil {
		t.Fatalf("notesList: %v", err)
	}
	q := mustQuery(t, transport)
	assertRequest(t, transport, "GET", "/api/v1/notes")
	assertHas(t, q, "filter_id", "17")
	assertHas(t, q, "start_date", "2026-05-01")
	assertHas(t, q, "end_date", "2026-05-31")

	ctx, transport = newTestCtx(t)
	if _, err := notesList(ctx, NotesListParams{}); err != nil {
		t.Fatalf("notesList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "filter_id")
	assertAbsent(t, q, "start_date")
	assertAbsent(t, q, "end_date")
}

func TestFiltersList_TypePassthrough(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := filtersList(ctx, FiltersListParams{Type: "deals"}); err != nil {
		t.Fatalf("filtersList: %v", err)
	}
	assertRequest(t, transport, "GET", "/api/v1/filters")
	assertHas(t, mustQuery(t, transport), "type", "deals")

	ctx, transport = newTestCtx(t)
	if _, err := filtersList(ctx, FiltersListParams{}); err != nil {
		t.Fatalf("filtersList zero: %v", err)
	}
	assertAbsent(t, mustQuery(t, transport), "type")
}

func TestDealsArchive_PatchesIsArchivedTrue(t *testing.T) {
	ctx, transport := newWriteTestCtx(t)
	if _, err := dealsArchive(ctx, DealsArchiveParams{ID: 7}); err != nil {
		t.Fatalf("dealsArchive: %v", err)
	}
	assertRequest(t, transport, "PATCH", "/api/v2/deals/7")
	body := mustBody(t, transport)
	if v, ok := body["is_archived"].(bool); !ok || !v {
		t.Errorf("is_archived = %v, want true", body["is_archived"])
	}
}

func TestDealsUnarchive_PatchesIsArchivedFalse(t *testing.T) {
	ctx, transport := newWriteTestCtx(t)
	if _, err := dealsUnarchive(ctx, DealsUnarchiveParams{ID: 7}); err != nil {
		t.Fatalf("dealsUnarchive: %v", err)
	}
	assertRequest(t, transport, "PATCH", "/api/v2/deals/7")
	body := mustBody(t, transport)
	if v, ok := body["is_archived"].(bool); !ok || v {
		t.Errorf("is_archived = %v, want false", body["is_archived"])
	}
}

func TestDealsArchive_WriteGate(t *testing.T) {
	ctx, _ := newTestCtx(t) // Config{} → write disabled
	res, err := dealsArchive(ctx, DealsArchiveParams{ID: 7})
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	d, ok := res.(disabledResult)
	if !ok || d.Error != "write_disabled" {
		t.Fatalf("expected write_disabled payload, got %+v", res)
	}
}

func TestDealsList_ArchivedRouting(t *testing.T) {
	yes := true
	ctx, transport := newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{IsArchived: &yes}); err != nil {
		t.Fatalf("dealsList archived: %v", err)
	}
	assertRequest(t, transport, "GET", "/api/v2/deals/archived")

	no := false
	ctx, transport = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{IsArchived: &no}); err != nil {
		t.Fatalf("dealsList non-archived: %v", err)
	}
	assertRequest(t, transport, "GET", "/api/v2/deals")

	ctx, transport = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{}); err != nil {
		t.Fatalf("dealsList default: %v", err)
	}
	assertRequest(t, transport, "GET", "/api/v2/deals")
}

func TestDealsUpdate_LabelIDsPropagation(t *testing.T) {
	labels := []int64{3, 9}
	ctx, transport := newWriteTestCtx(t)
	if _, err := dealsUpdate(ctx, DealsUpdateParams{ID: 5, LabelIDs: &labels}); err != nil {
		t.Fatalf("dealsUpdate: %v", err)
	}
	assertRequest(t, transport, "PATCH", "/api/v2/deals/5")
	body := mustBody(t, transport)
	got, ok := body["label_ids"].([]any)
	if !ok || len(got) != 2 || got[0].(float64) != 3 || got[1].(float64) != 9 {
		t.Errorf("label_ids = %v, want [3 9]", body["label_ids"])
	}

	// Empty (non-nil) slice must be sent — it clears all labels.
	empty := []int64{}
	ctx, transport = newWriteTestCtx(t)
	if _, err := dealsUpdate(ctx, DealsUpdateParams{ID: 5, LabelIDs: &empty}); err != nil {
		t.Fatalf("dealsUpdate clear: %v", err)
	}
	body = mustBody(t, transport)
	if got, ok := body["label_ids"].([]any); !ok || len(got) != 0 {
		t.Errorf("label_ids = %v, want []", body["label_ids"])
	}

	// Nil pointer must be absent.
	ctx, transport = newWriteTestCtx(t)
	if _, err := dealsUpdate(ctx, DealsUpdateParams{ID: 5, Title: "t"}); err != nil {
		t.Fatalf("dealsUpdate nil: %v", err)
	}
	body = mustBody(t, transport)
	if _, present := body["label_ids"]; present {
		t.Errorf("label_ids should be absent when not provided, body: %v", body)
	}
}
