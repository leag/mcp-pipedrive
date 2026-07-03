# Pipedrive MCP Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 13 Pipedrive-MCP gaps from mary's TODO.md — 9 new tools, 5 changed tools, docs — in six grouped commits on `feat/mary-todo-gaps`.

**Architecture:** Every tool follows the repo's existing three-part pattern: a params struct with `json` + `jsonschema` tags, a handler `func(ctx, args) (any, error)` that opens with `ensureToolAllowed` + `clientOrError`, and a package-level `var X = mcppipedrive.MustTool(...)` appended to `allTools()` in `tools/registry.go`. Reads go through `client.CachedGet`; writes use `client.DoJSON` then invalidate the relevant cache prefix.

**Tech Stack:** Go 1.25 (stdlib `testing` only — no testify), `mark3labs/mcp-go`, `invopop/jsonschema` (reflection from params structs), bbolt cache.

**Spec:** `docs/superpowers/specs/2026-07-03-pipedrive-mcp-gaps-design.md` (already committed on this branch). API facts in this plan were verified against Pipedrive's official OpenAPI specs on 2026-07-03; copies live at `/home/luisatala/.claude-prey/jobs/18e51d01/tmp/pd-v1.yaml` and `pd-v2.yaml` if you need to re-check anything.

## Global Constraints

- Repo: `/home/luisatala/projects/mcp-pipedrive`, branch `feat/mary-todo-gaps`. All paths below are repo-relative.
- Tool names are dotted (`pipedrive.deals.archive`); the SAME string goes into `MustTool` and `ensureToolAllowed` — they must match exactly.
- Write tools use `guardWrite`; delete tools use `guardDelete` (needs BOTH `PIPEDRIVE_ALLOW_WRITE` and `PIPEDRIVE_ALLOW_DELETE`); read tools use `guardRead` + take a `cache_mode` param.
- Every new tool `var` MUST be appended to `allTools()` in `tools/registry.go` or it is never exposed.
- Handlers return soft errors via `wrapAPIError(err)` (defined in `tools/deals.go:434`) so the LLM sees upstream validation details.
- Raw payloads echoed to the caller go through `internal.MaskSensitive`.
- Run `go build ./... && go test ./...` before every commit; all green.
- Commit messages: end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

---

### Task 1: Deal lifecycle — archive/unarchive tools, archived listing, label/archived fields on responses

**Files:**
- Modify: `tools/deals.go` (params + handlers + tool vars)
- Modify: `pipedrive/normalize.go:5-48` (`NormalizedDeal`, `NormalizeDeal`)
- Modify: `tools/registry.go:41` (deals core line)
- Modify: `tools/list_params_query_test.go:17-46` (extend `captureTransport` with body capture, add write-ctx helper)
- Test: `tools/list_params_query_test.go`, `pipedrive/normalize_test.go`, `tools/registry_test.go`

**Interfaces:**
- Consumes: `ensureToolAllowed`, `clientOrError`, `wrapAPIError`, `invalidateDealsCache`, `extractItemData` (all existing, `tools/deals.go` + `tools/common.go`).
- Produces: `dealsArchive`/`dealsUnarchive` handlers, `DealsArchive`/`DealsUnarchive` tool vars, `DealsListParams.IsArchived *bool`, `NormalizedDeal.LabelIDs []int64` + `NormalizedDeal.IsArchived bool`, test helpers `captureTransport.lastBody []byte` and `newWriteTestCtx(t)` used by Tasks 2, 4, 5.

**Verified API facts:** There are NO `POST /deals/{id}/archive` endpoints. Archive = v2 `PATCH /deals/{id}` with body `{"is_archived": true}` (Pipedrive sets `archive_time` automatically). Unarchive = same with `false`. `GET /deals` returns only non-archived deals and has NO `is_archived` query param; archived deals come from `GET /deals/archived` with the identical param set. The v2 Deal schema includes `label_ids` (int array) and `is_archived` (bool).

- [ ] **Step 1: Extend the test transport to capture request bodies and add a write-enabled context helper**

In `tools/list_params_query_test.go`, replace the `captureTransport` struct and `RoundTrip` (lines 17-31) with:

```go
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
```

(`bytes`, `io` are already imported in this file.)

Below `newTestCtx` (line 46), add:

```go
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
```

Add `"encoding/json"` to the file's imports.

- [ ] **Step 2: Write the failing tests for archive/unarchive and archived listing**

Append to `tools/list_params_query_test.go`:

```go
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
```

Append to `pipedrive/normalize_test.go`:

```go
func TestNormalizeDeal_LabelIDsAndIsArchived(t *testing.T) {
	d := NormalizeDeal(map[string]any{
		"id":          float64(1),
		"title":       "x",
		"status":      "open",
		"label_ids":   []any{float64(3), float64(9)},
		"is_archived": true,
	})
	if len(d.LabelIDs) != 2 || d.LabelIDs[0] != 3 || d.LabelIDs[1] != 9 {
		t.Errorf("LabelIDs = %v, want [3 9]", d.LabelIDs)
	}
	if !d.IsArchived {
		t.Error("IsArchived = false, want true")
	}

	empty := NormalizeDeal(map[string]any{"id": float64(2), "title": "y", "status": "open"})
	if empty.LabelIDs != nil {
		t.Errorf("LabelIDs = %v, want nil when absent", empty.LabelIDs)
	}
	if empty.IsArchived {
		t.Error("IsArchived = true, want false when absent")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./tools/ ./pipedrive/ -run 'DealsArchive|DealsUnarchive|ArchivedRouting|LabelIDsAndIsArchived' -v`
Expected: compile FAILURE — `undefined: dealsArchive`, `undefined: DealsArchiveParams`, `d.LabelIDs undefined`.

- [ ] **Step 4: Implement**

(a) In `pipedrive/normalize.go`, add two fields to `NormalizedDeal` (after `StageID`, line 15):

```go
	LabelIDs       []int64        `json:"label_ids,omitempty"`
	IsArchived     bool           `json:"is_archived,omitempty"`
```

In `NormalizeDeal`, after the `custom_fields` block (line 46), add:

```go
	if arr, ok := raw["label_ids"].([]any); ok {
		for _, v := range arr {
			if id := toInt64(v); id != 0 {
				d.LabelIDs = append(d.LabelIDs, id)
			}
		}
	}
	d.IsArchived = toBool(raw["is_archived"])
```

(b) In `tools/deals.go`, add to `DealsListParams` (after `FilterID`, line 30):

```go
	IsArchived   *bool  `json:"is_archived,omitempty" jsonschema:"description=true → list archived deals (targets GET /deals/archived); false or omitted → non-archived deals"`
```

In `dealsList`, replace the fixed `"/deals"` path (lines 128 and 132) with:

```go
	path := "/deals"
	if args.IsArchived != nil && *args.IsArchived {
		path = "/deals/archived"
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, q, nil)
```

(c) Add params after `DealsDeleteParams` (line 81):

```go
type DealsArchiveParams struct {
	ID int64 `json:"id" jsonschema:"description=Deal ID to archive"`
}

type DealsUnarchiveParams struct {
	ID int64 `json:"id" jsonschema:"description=Deal ID to unarchive"`
}
```

Add handlers after `dealsDelete` (line 360). The v2 API has no dedicated archive
endpoint; archiving is a PATCH on `is_archived`:

```go
func dealsSetArchived(ctx context.Context, toolName string, id int64, archived bool) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, toolName, guardWrite); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if id <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	path := "/deals/" + strconv.FormatInt(id, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, map[string]any{"is_archived": archived})
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateDealsCache(client, id)
	raw, dealRaw := extractItemData(payload)
	deal := pipedrive.NormalizeDeal(dealRaw)
	return internal.Wrap(map[string]any{"deal": deal, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func dealsArchive(ctx context.Context, args DealsArchiveParams) (any, error) {
	return dealsSetArchived(ctx, "pipedrive.deals.archive", args.ID, true)
}

func dealsUnarchive(ctx context.Context, args DealsUnarchiveParams) (any, error) {
	return dealsSetArchived(ctx, "pipedrive.deals.unarchive", args.ID, false)
}
```

(d) Add tool vars after `DealsDelete` (line 505):

```go
var DealsArchive = mcppipedrive.MustTool(
	"pipedrive.deals.archive",
	"Archive a deal (write; sets is_archived=true via PATCH). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealsArchive,
	mcp.WithTitleAnnotation("Archive deal"),
	mcp.WithIdempotentHintAnnotation(true),
)

var DealsUnarchive = mcppipedrive.MustTool(
	"pipedrive.deals.unarchive",
	"Unarchive a deal (write; sets is_archived=false via PATCH). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealsUnarchive,
	mcp.WithTitleAnnotation("Unarchive deal"),
	mcp.WithIdempotentHintAnnotation(true),
)
```

(e) Update the `DealsList` tool description (line 460) to:

```go
	"List deals (Pipedrive API v2) with optional filter_id, status, owner/pipeline/stage, updated_since/updated_until, sort_by/sort_direction, is_archived, and cursor pagination. When paginating a filtered call, re-pass filter_id with every cursor.",
```

(Note: `sort_by`/`sort_direction` land in Task 4; writing the final description now avoids touching the string twice.)

(f) In `tools/registry.go` line 41, extend the deals core line:

```go
		DealsList, DealsGet, DealsSearch, DealsCreate, DealsUpdate, DealsDelete, DealsArchive, DealsUnarchive,
```

(g) In `tools/registry_test.go`, add `"pipedrive.deals.archive"` and `"pipedrive.deals.unarchive"` to the `must` list in `TestRegistry_DefaultRegistersEverythingExceptDestructiveAdmin` (line 62).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./tools/ ./pipedrive/`
Expected: PASS (all packages).

- [ ] **Step 6: Commit**

```bash
git add tools/deals.go tools/registry.go tools/registry_test.go tools/list_params_query_test.go pipedrive/normalize.go pipedrive/normalize_test.go
git commit -m "feat(deals): archive/unarchive tools, archived-deal listing, label_ids/is_archived on responses

Pipedrive v2 has no dedicated archive endpoints (verified against the
official OpenAPI spec): archiving is PATCH /deals/{id} is_archived, and
archived deals are listed via GET /deals/archived. Closes the mary
TODO.md archive workaround (mary_archived_at custom-field stamping).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: First-class labels on deals.update

**Files:**
- Modify: `tools/deals.go` (`DealsUpdateParams`, `dealsUpdate`, `DealsUpdate` description)
- Test: `tools/list_params_query_test.go`

**Interfaces:**
- Consumes: `newWriteTestCtx`, `mustBody` (Task 1), existing `dealsUpdate` handler.
- Produces: `DealsUpdateParams.LabelIDs *[]int64`.

- [ ] **Step 1: Write the failing test**

Append to `tools/list_params_query_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tools/ -run TestDealsUpdate_LabelIDs -v`
Expected: compile FAILURE — `unknown field LabelIDs in struct literal`.

- [ ] **Step 3: Implement**

In `tools/deals.go`, add to `DealsUpdateParams` (after `Status`, line 75):

```go
	LabelIDs       *[]int64       `json:"label_ids,omitempty" jsonschema:"description=Replace the deal's labels with these label option IDs (empty array clears all labels). Resolve option IDs from the deal label field options in pipedrive.context.get deal_fields"`
```

In `dealsUpdate`, after the `CustomFields` block (line 311), add:

```go
	if args.LabelIDs != nil {
		body["label_ids"] = *args.LabelIDs
	}
```

Update the `DealsUpdate` description (line 494) to:

```go
	"Update an existing deal (write), including label_ids for deal labels. Requires PIPEDRIVE_ALLOW_WRITE=true.",
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/deals.go tools/list_params_query_test.go
git commit -m "feat(deals): first-class label_ids on deals.update

Replaces the custom_fields smuggling workaround for deal labels.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Metadata — activity types in context.get, activities.create guidance, deal_fields.add_option

**Files:**
- Modify: `tools/context.go` (calls slice + description)
- Modify: `tools/activities.go` (`ActivitiesCreate` description only)
- Create: `tools/deal_fields.go`
- Create: `tools/deal_fields_test.go` (includes the shared `scriptedTransport` helper used again in Task 5)
- Modify: `tools/registry.go`, `tools/registry_test.go`
- Test: `tools/context_test.go` (new)

**Interfaces:**
- Consumes: `ensureToolAllowed`, `clientOrError`, `wrapAPIError`, `extractItemData`, `toStringRaw` (`tools/mailbox.go:589`).
- Produces: `context.get` result gains `activity_types` key; `dealFieldsAddOption` handler + `DealFieldsAddOption` tool var; test helper `scriptedTransport` (multi-response, thread-safe) used by Task 5.

**Verified API facts:** v1 `GET /activityTypes` exists, items carry `id`, `name`, `key_string`, `icon_key`, `active_flag`, `is_custom_flag`. v1 `PUT /dealFields/{id}` accepts only `{name?, options?, add_visible_flag?}`; `options` is a FULL-ARRAY REPLACE — existing options must be resent with their `id`, new options are `{label}` only. Option-bearing field types are `enum` (single) and `set` (multi).

- [ ] **Step 1: Write the failing tests**

Create `tools/context_test.go`:

```go
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
```

And add this small accessor to `tools/list_params_query_test.go` (below `newWriteTestCtx`):

```go
// clientFromTestCtx returns the *pipedrive.Client installed by newTestCtx so
// tests can adjust its transport.
func clientFromTestCtx(t *testing.T, ctx context.Context) *pipedrive.Client {
	t.Helper()
	client := pipedrive.ClientFromContext(ctx)
	if client == nil {
		t.Fatal("no client in test context")
	}
	return client
}
```

Create `tools/deal_fields_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tools/ -run 'ContextGet_FetchesActivityTypes|DealFieldsAddOption' -v`
Expected: compile FAILURE — `undefined: dealFieldsAddOption`, `undefined: DealFieldsAddOptionParams`, `undefined: clientFromTestCtx`.

- [ ] **Step 3: Implement**

(a) In `tools/context.go`, add a fifth row to the `calls` slice (line 54):

```go
		{pipedrive.V1, "/activityTypes", nil, "activity_types"},
```

Update the doc comment (lines 20-21) to `// contextGet returns users + pipelines + stages + deal-fields + activity-types metadata in one call. All five sub-calls share the metadata TTL.` and the `ContextGet` description (line 99) to:

```go
	"Fetch users, pipelines, stages, deal-field and activity-type metadata in one cached call. Useful to prime an LLM before deal operations.",
```

(b) In `tools/activities.go`, update the `ActivitiesCreate` description (line 306) to:

```go
	"Create a new activity (write). Requires PIPEDRIVE_ALLOW_WRITE=true. Validate the type key against pipedrive.context.get activity_types[].key_string first — unknown types are rejected by Pipedrive.",
```

(c) Create `tools/deal_fields.go`:

```go
package tools

// Deal-field option management. Pipedrive's v1 PUT /dealFields/{id} replaces
// the FULL options array: existing options must be resent with their id, new
// options carry only a label. This tool wraps the read-merge-write cycle so
// callers can't accidentally drop existing options.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type DealFieldsAddOptionParams struct {
	FieldID int64    `json:"field_id" jsonschema:"description=Numeric deal field ID (from pipedrive.context.get deal_fields)"`
	Options []string `json:"options" jsonschema:"description=Option labels to append to the enum/set field (existing labels are skipped)"`
}

func dealFieldsAddOption(ctx context.Context, args DealFieldsAddOptionParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.deal_fields.add_option", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if args.FieldID <= 0 {
		return nil, fmt.Errorf("field_id is required and must be > 0")
	}
	if len(args.Options) == 0 {
		return nil, fmt.Errorf("options must contain at least one label")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	path := "/dealFields/" + strconv.FormatInt(args.FieldID, 10)
	getReq, err := client.NewRequest(pipedrive.V1, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var getPayload any
	if err := client.DoJSON(getReq.WithContext(ctx), &getPayload); err != nil {
		return nil, wrapAPIError(err)
	}
	_, field := extractItemData(getPayload)
	fieldType := toStringRaw(field["field_type"])
	if fieldType != "enum" && fieldType != "set" {
		return nil, fmt.Errorf("field %d has field_type %q; options can only be added to enum or set fields", args.FieldID, fieldType)
	}

	existing, _ := field["options"].([]any)
	merged := make([]any, 0, len(existing)+len(args.Options))
	have := map[string]bool{}
	for _, o := range existing {
		om, ok := o.(map[string]any)
		if !ok {
			continue
		}
		label := toStringRaw(om["label"])
		have[label] = true
		merged = append(merged, map[string]any{"id": om["id"], "label": label})
	}
	added := []string{}
	skipped := []string{}
	for _, label := range args.Options {
		if have[label] {
			skipped = append(skipped, label)
			continue
		}
		have[label] = true
		added = append(added, label)
		merged = append(merged, map[string]any{"label": label})
	}
	if len(added) == 0 {
		return internal.Wrap(map[string]any{
			"field":   internal.MaskSensitive(field),
			"added":   added,
			"skipped": skipped,
		}, nil), nil
	}

	putReq, err := client.NewRequest(pipedrive.V1, http.MethodPut, path, nil, map[string]any{"options": merged})
	if err != nil {
		return nil, err
	}
	var putPayload any
	if err := client.DoJSON(putReq.WithContext(ctx), &putPayload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/dealFields")
	_, updated := extractItemData(putPayload)
	return internal.Wrap(map[string]any{
		"field":   internal.MaskSensitive(updated),
		"added":   added,
		"skipped": skipped,
	}, nil), nil
}

var DealFieldsAddOption = mcppipedrive.MustTool(
	"pipedrive.deal_fields.add_option",
	"Append option labels to an enum/set deal field (write; read-merge-write on v1 /dealFields). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealFieldsAddOption,
	mcp.WithTitleAnnotation("Add deal-field option"),
)
```

(d) In `tools/registry.go`, add a new section after the filters line (line 63):

```go
		// deal fields (option management)
		DealFieldsAddOption,
```

(e) In `tools/registry_test.go`, add `"pipedrive.deal_fields.add_option"` to the `must` list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/context.go tools/activities.go tools/deal_fields.go tools/deal_fields_test.go tools/context_test.go tools/list_params_query_test.go tools/registry.go tools/registry_test.go
git commit -m "feat(metadata): activity types in context.get, deal_fields.add_option tool

context.get now aggregates GET /activityTypes so callers can validate
activity type keys upfront instead of the blind email→task fallback.
deal_fields.add_option wraps Pipedrive's full-array-replace options PUT
in a read-merge-write cycle.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Listing/filtering — sort params on deals.list, filters.create/update

**Files:**
- Modify: `tools/deals.go` (`DealsListParams`, `dealsList`)
- Modify: `tools/filters.go` (create/update params + handlers + vars)
- Modify: `tools/registry.go`, `tools/registry_test.go`
- Test: `tools/list_params_query_test.go`

**Interfaces:**
- Consumes: `newWriteTestCtx`, `mustBody` (Task 1); `internal.RequireOneOf` (`internal/validate.go:12`); `pipedrive.NormalizeFilter` (`pipedrive/normalize.go:339`).
- Produces: `DealsListParams.SortBy`/`SortDirection`; `filtersCreate`/`filtersUpdate` handlers, `FiltersCreate`/`FiltersUpdate` tool vars.

**Verified API facts:** v2 `GET /deals` `sort_by` enum is exactly `id | update_time | add_time` (default `id`); `sort_direction` is `asc | desc`. There are NO `add_since`/`add_until` params — creation-date scans use `sort_by=add_time` + cursor and stop at the cutoff. v1 `POST /filters` requires `name`, `type`, `conditions`; `type` enum: `deals|leads|org|people|products|activity|projects`. v1 `PUT /filters/{id}` accepts ONLY `name` and `conditions` (`conditions` required, `type` immutable). `conditions` is a two-level glue tree, max 16 leaf conditions.

- [ ] **Step 1: Write the failing tests**

Append to `tools/list_params_query_test.go`:

```go
func TestDealsList_SortPropagation(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{SortBy: "add_time", SortDirection: "desc"}); err != nil {
		t.Fatalf("dealsList: %v", err)
	}
	q := mustQuery(t, transport)
	assertHas(t, q, "sort_by", "add_time")
	assertHas(t, q, "sort_direction", "desc")

	ctx, transport = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{}); err != nil {
		t.Fatalf("dealsList zero: %v", err)
	}
	q = mustQuery(t, transport)
	assertAbsent(t, q, "sort_by")
	assertAbsent(t, q, "sort_direction")

	ctx, _ = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{SortBy: "stage_id"}); err == nil {
		t.Fatal("expected error for invalid sort_by")
	}
	ctx, _ = newTestCtx(t)
	if _, err := dealsList(ctx, DealsListParams{SortDirection: "down"}); err == nil {
		t.Fatal("expected error for invalid sort_direction")
	}
}

func TestFiltersCreate_BodyPropagation(t *testing.T) {
	conditions := map[string]any{
		"glue": "and",
		"conditions": []any{
			map[string]any{"glue": "and", "conditions": []any{}},
			map[string]any{"glue": "or", "conditions": []any{}},
		},
	}
	ctx, transport := newWriteTestCtx(t)
	if _, err := filtersCreate(ctx, FiltersCreateParams{
		Name: "mary sweep", Type: "deals", Conditions: conditions,
	}); err != nil {
		t.Fatalf("filtersCreate: %v", err)
	}
	assertRequest(t, transport, "POST", "/api/v1/filters")
	body := mustBody(t, transport)
	if body["name"] != "mary sweep" || body["type"] != "deals" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["conditions"].(map[string]any); !ok {
		t.Errorf("conditions not passed through: %v", body["conditions"])
	}

	ctx, _ = newWriteTestCtx(t)
	if _, err := filtersCreate(ctx, FiltersCreateParams{Name: "x", Type: "bogus", Conditions: conditions}); err == nil {
		t.Fatal("expected error for invalid type")
	}
	ctx, _ = newWriteTestCtx(t)
	if _, err := filtersCreate(ctx, FiltersCreateParams{Name: "x", Type: "deals"}); err == nil {
		t.Fatal("expected error for missing conditions")
	}
}

func TestFiltersUpdate_BodyPropagation(t *testing.T) {
	conditions := map[string]any{"glue": "and", "conditions": []any{}}
	ctx, transport := newWriteTestCtx(t)
	if _, err := filtersUpdate(ctx, FiltersUpdateParams{ID: 12, Name: "renamed", Conditions: conditions}); err != nil {
		t.Fatalf("filtersUpdate: %v", err)
	}
	assertRequest(t, transport, "PUT", "/api/v1/filters/12")
	body := mustBody(t, transport)
	if body["name"] != "renamed" {
		t.Errorf("name = %v", body["name"])
	}
	if _, ok := body["conditions"].(map[string]any); !ok {
		t.Errorf("conditions not passed through: %v", body["conditions"])
	}
	if _, present := body["type"]; present {
		t.Errorf("type must never be sent on update: %v", body)
	}

	ctx, _ = newWriteTestCtx(t)
	if _, err := filtersUpdate(ctx, FiltersUpdateParams{ID: 12}); err == nil {
		t.Fatal("expected error for missing conditions (required by v1 PUT)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tools/ -run 'SortPropagation|FiltersCreate|FiltersUpdate' -v`
Expected: compile FAILURE — `unknown field SortBy`, `undefined: filtersCreate`, `undefined: FiltersCreateParams`.

- [ ] **Step 3: Implement**

(a) In `tools/deals.go`, add to `DealsListParams` (after `UpdatedUntil`, line 32):

```go
	SortBy        string `json:"sort_by,omitempty" jsonschema:"description=Sort field: id|update_time|add_time (default id). Use add_time+desc to scan by creation date"`
	SortDirection string `json:"sort_direction,omitempty" jsonschema:"description=Sort direction: asc|desc (default asc)"`
```

In `dealsList`, after the `UpdatedUntil` block (line 126), add:

```go
	if args.SortBy != "" {
		if err := internal.RequireOneOf(args.SortBy, "sort_by", "id", "update_time", "add_time"); err != nil {
			return nil, err
		}
		q.Set("sort_by", args.SortBy)
	}
	if args.SortDirection != "" {
		if err := internal.RequireOneOf(args.SortDirection, "sort_direction", "asc", "desc"); err != nil {
			return nil, err
		}
		q.Set("sort_direction", args.SortDirection)
	}
```

(b) In `tools/filters.go`, add imports `"fmt"` and `"strconv"`, then append:

```go
type FiltersCreateParams struct {
	Name       string         `json:"name" jsonschema:"description=Filter name"`
	Type       string         `json:"type" jsonschema:"description=Filter type: deals|leads|org|people|products|activity|projects"`
	Conditions map[string]any `json:"conditions" jsonschema:"description=Pipedrive conditions tree (two-level glue structure with max 16 leaf conditions): {\"glue\":\"and\",\"conditions\":[{\"glue\":\"and\",\"conditions\":[{\"object\":\"deal\",\"field_id\":\"12345\",\"operator\":\"=\",\"value\":\"open\"}]},{\"glue\":\"or\",\"conditions\":[]}]}. Resolve field_id values from pipedrive.context.get deal_fields"`
}

type FiltersUpdateParams struct {
	ID         int64          `json:"id" jsonschema:"description=Filter ID to update (discover via pipedrive.filters.list)"`
	Name       string         `json:"name,omitempty" jsonschema:"description=New filter name"`
	Conditions map[string]any `json:"conditions" jsonschema:"description=Full replacement conditions tree (required by the v1 API on every update). The filter's type cannot be changed"`
}

var filterTypes = []string{"deals", "leads", "org", "people", "products", "activity", "projects"}

func filtersCreate(ctx context.Context, args FiltersCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.filters.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Name, "name"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.Type, "type", filterTypes...); err != nil {
		return nil, err
	}
	if len(args.Conditions) == 0 {
		return nil, fmt.Errorf("conditions is required (Pipedrive glue tree, see tool description)")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"name": args.Name, "type": args.Type, "conditions": args.Conditions}
	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/filters", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/filters")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"filter": pipedrive.NormalizeFilter(m),
		"raw":    internal.MaskSensitive(raw),
	}, nil), nil
}

func filtersUpdate(ctx context.Context, args FiltersUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.filters.update", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	if len(args.Conditions) == 0 {
		return nil, fmt.Errorf("conditions is required — the v1 PUT replaces the whole tree on every update")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"conditions": args.Conditions}
	if args.Name != "" {
		body["name"] = args.Name
	}
	path := "/filters/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V1, http.MethodPut, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/filters")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"filter": pipedrive.NormalizeFilter(m),
		"raw":    internal.MaskSensitive(raw),
	}, nil), nil
}

var FiltersCreate = mcppipedrive.MustTool("pipedrive.filters.create",
	"Create a saved filter (write; v1). Combine with filter_id on list tools for server-side filtering, including custom-field conditions. Requires PIPEDRIVE_ALLOW_WRITE=true. See docs/filters.md.",
	filtersCreate,
	mcp.WithTitleAnnotation("Create saved filter"))

var FiltersUpdate = mcppipedrive.MustTool("pipedrive.filters.update",
	"Update a saved filter's name/conditions (write; v1; type is immutable, conditions fully replaced). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	filtersUpdate,
	mcp.WithTitleAnnotation("Update saved filter"))
```

(c) In `tools/registry.go`, extend the filters line (line 63):

```go
		// filters (saved-filter discovery + management)
		FiltersList, FiltersCreate, FiltersUpdate,
```

(d) Add `"pipedrive.filters.create"` and `"pipedrive.filters.update"` to the registry test's `must` list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/deals.go tools/filters.go tools/registry.go tools/registry_test.go tools/list_params_query_test.go
git commit -m "feat(filtering): sort_by/sort_direction on deals.list, filters.create/update

sort_by=add_time+desc enables creation-date scans (v2 has no add_time
range params). Saved-filter management gives true server-side filtering
on custom fields via filter_id, replacing full-list + in-memory sweeps;
this also covers the deals.search structured-filter gap.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Throughput/eventing — activities.batch_create, webhooks list/create/delete

**Files:**
- Modify: `tools/activities.go` (extract `activityCreateBody` helper)
- Create: `tools/activities_batch.go`
- Create: `tools/webhooks.go`
- Modify: `pipedrive/normalize.go` (NormalizedWebhook)
- Modify: `tools/registry.go`, `tools/registry_test.go`
- Test: `tools/activities_batch_test.go` (new), `tools/list_params_query_test.go`

**Interfaces:**
- Consumes: `scriptedTransport`/`newScriptedCtx` (Task 3), `newWriteTestCtx`/`mustBody` (Task 1), `invalidateActivitiesCache` (`tools/activities.go:282`), `internal.RequireID`/`RequireOneOf`.
- Produces: `activityCreateBody(args ActivitiesCreateParams) map[string]any`; `activitiesBatchCreate`, `webhooksList`/`webhooksCreate`/`webhooksDelete` handlers; `ActivitiesBatchCreate`, `WebhooksList`, `WebhooksCreate`, `WebhooksDelete` tool vars; `pipedrive.NormalizeWebhook`/`NormalizeWebhookList`.

**Verified API facts:** Pipedrive has NO batch-create endpoint — the batch tool loops v2 `POST /activities` server-side (the client's rate limiter applies per request). v1 `POST /webhooks` REQUIRES `name` (≤255 chars), `subscription_url`, `event_action` (`create|change|delete|*`), `event_object` (`activity|deal|lead|note|organization|person|pipeline|product|stage|user|*`); optional `user_id`, `http_auth_user`, `http_auth_password`, `version` (`"1.0"|"2.0"`, default `"2.0"`). `DELETE /webhooks/{id}` takes only the path id. Webhook objects carry `is_active` (0/1). v2 has no webhooks endpoints.

- [ ] **Step 1: Extract the body builder in `tools/activities.go`**

Replace the body-building block in `activitiesCreate` (lines 186-198) with a call to a new helper, added just above `activitiesCreate`:

```go
// activityCreateBody maps create params to the v2 POST /activities body.
// Shared by activities.create and activities.batch_create.
func activityCreateBody(args ActivitiesCreateParams) map[string]any {
	body := map[string]any{"subject": args.Subject}
	setIfNonZero(body, "type", args.Type)
	setIfNonZero(body, "due_date", args.DueDate)
	setIfNonZero(body, "due_time", args.DueTime)
	setIfNonZero(body, "duration", args.Duration)
	if args.Done {
		body["done"] = true
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "deal_id", args.DealID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZero(body, "note", args.Note)
	return body
}
```

and in `activitiesCreate`:

```go
	body := activityCreateBody(args)
```

Run: `go test ./tools/` — expected PASS (pure refactor; `TestActivitiesList_QueryPropagation` and friends still green).

- [ ] **Step 2: Write the failing batch tests**

Create `tools/activities_batch_test.go`:

```go
package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActivitiesBatchCreate_ContinuesOnError(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":1,"subject":"a"}}`,
		`ERR:{"success":false,"error":"bad type"}`,
		`{"success":true,"data":{"id":3,"subject":"c"}}`,
	})
	res, err := activitiesBatchCreate(ctx, ActivitiesBatchCreateParams{
		Activities: []ActivitiesCreateParams{
			{Subject: "a"}, {Subject: "b", Type: "bogus"}, {Subject: "c"},
		},
	})
	if err != nil {
		t.Fatalf("activitiesBatchCreate: %v", err)
	}
	if len(transport.requests) != 3 {
		t.Fatalf("expected 3 POSTs, got %d", len(transport.requests))
	}
	for i, req := range transport.requests {
		if req.Method != "POST" || req.URL.Path != "/api/v2/activities" {
			t.Errorf("request %d = %s %s, want POST /api/v2/activities", i, req.Method, req.URL.Path)
		}
	}
	out, _ := json.Marshal(res)
	s := string(out)
	if !strings.Contains(s, `"succeeded":2`) || !strings.Contains(s, `"failed":1`) {
		t.Errorf("expected succeeded=2 failed=1, got %s", s)
	}
	if !strings.Contains(s, "bad type") {
		t.Errorf("per-item error must carry the upstream message, got %s", s)
	}
}

func TestActivitiesBatchCreate_StopOnError(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":1,"subject":"a"}}`,
		`ERR:{"success":false,"error":"boom"}`,
	})
	res, err := activitiesBatchCreate(ctx, ActivitiesBatchCreateParams{
		StopOnError: true,
		Activities: []ActivitiesCreateParams{
			{Subject: "a"}, {Subject: "b"}, {Subject: "c"},
		},
	})
	if err != nil {
		t.Fatalf("activitiesBatchCreate: %v", err)
	}
	if len(transport.requests) != 2 {
		t.Fatalf("stop_on_error must halt after the failure; got %d requests", len(transport.requests))
	}
	out, _ := json.Marshal(res)
	if !strings.Contains(string(out), `"succeeded":1`) || !strings.Contains(string(out), `"failed":1`) {
		t.Errorf("expected succeeded=1 failed=1, got %s", out)
	}
}

func TestActivitiesBatchCreate_Validation(t *testing.T) {
	ctx, _ := newScriptedCtx(t, nil)
	if _, err := activitiesBatchCreate(ctx, ActivitiesBatchCreateParams{}); err == nil {
		t.Fatal("expected error for empty activities")
	}
	tooMany := make([]ActivitiesCreateParams, 101)
	for i := range tooMany {
		tooMany[i] = ActivitiesCreateParams{Subject: "x"}
	}
	if _, err := activitiesBatchCreate(ctx, ActivitiesBatchCreateParams{Activities: tooMany}); err == nil {
		t.Fatal("expected error for >100 items")
	}
}
```

Append webhook tests to `tools/list_params_query_test.go`:

```go
func TestWebhooksCreate_BodyPropagation(t *testing.T) {
	ctx, transport := newWriteTestCtx(t)
	if _, err := webhooksCreate(ctx, WebhooksCreateParams{
		Name:            "n8n new deal",
		SubscriptionURL: "https://n8n.example.com/webhook/abc",
		EventAction:     "create",
		EventObject:     "deal",
	}); err != nil {
		t.Fatalf("webhooksCreate: %v", err)
	}
	assertRequest(t, transport, "POST", "/api/v1/webhooks")
	body := mustBody(t, transport)
	if body["name"] != "n8n new deal" || body["subscription_url"] != "https://n8n.example.com/webhook/abc" {
		t.Errorf("body = %v", body)
	}
	if body["event_action"] != "create" || body["event_object"] != "deal" {
		t.Errorf("event fields = %v / %v", body["event_action"], body["event_object"])
	}

	ctx, _ = newWriteTestCtx(t)
	if _, err := webhooksCreate(ctx, WebhooksCreateParams{
		Name: "x", SubscriptionURL: "https://x", EventAction: "created", EventObject: "deal",
	}); err == nil {
		t.Fatal("expected error for invalid event_action (v1 uses create|change|delete|*)")
	}
}

func TestWebhooksList_And_Delete(t *testing.T) {
	ctx, transport := newTestCtx(t)
	if _, err := webhooksList(ctx, WebhooksListParams{}); err != nil {
		t.Fatalf("webhooksList: %v", err)
	}
	assertRequest(t, transport, "GET", "/api/v1/webhooks")

	ctx, transport = newWriteTestCtx(t)
	if _, err := webhooksDelete(ctx, WebhooksDeleteParams{ID: 4}); err != nil {
		t.Fatalf("webhooksDelete: %v", err)
	}
	assertRequest(t, transport, "DELETE", "/api/v1/webhooks/4")

	// Delete is gated on BOTH flags — write-only must refuse.
	ctx, _ = newTestCtx(t)
	ctx = pipedrive.WithConfig(ctx, pipedrive.Config{AllowWrite: true})
	res, err := webhooksDelete(ctx, WebhooksDeleteParams{ID: 4})
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if d, ok := res.(disabledResult); !ok || d.Error != "delete_disabled" {
		t.Fatalf("expected delete_disabled, got %+v", res)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./tools/ -run 'BatchCreate|Webhooks' -v`
Expected: compile FAILURE — `undefined: activitiesBatchCreate`, `undefined: webhooksCreate`, etc.

- [ ] **Step 4: Implement**

(a) Create `tools/activities_batch.go`:

```go
package tools

// Batch activity creation. Pipedrive has no batch endpoint, so this loops
// POST /activities server-side: one MCP call instead of N round-trips
// through the LLM. The client's rate limiter still applies per request.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

const batchCreateMax = 100

type ActivitiesBatchCreateParams struct {
	Activities  []ActivitiesCreateParams `json:"activities" jsonschema:"description=Activities to create, in order (1-100 items; same fields as pipedrive.activities.create)"`
	StopOnError bool                     `json:"stop_on_error,omitempty" jsonschema:"description=Abort at the first failed item (default false: continue and report per-item errors)"`
}

type batchItemResult struct {
	Index    int    `json:"index"`
	OK       bool   `json:"ok"`
	Activity any    `json:"activity,omitempty"`
	Error    string `json:"error,omitempty"`
}

func activitiesBatchCreate(ctx context.Context, args ActivitiesBatchCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.batch_create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if len(args.Activities) == 0 {
		return nil, fmt.Errorf("activities must contain at least one item")
	}
	if len(args.Activities) > batchCreateMax {
		return nil, fmt.Errorf("activities is limited to %d items per call, got %d", batchCreateMax, len(args.Activities))
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]batchItemResult, 0, len(args.Activities))
	succeeded, failed := 0, 0
	for i, item := range args.Activities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item.Subject == "" {
			failed++
			results = append(results, batchItemResult{Index: i, Error: "subject is required"})
			if args.StopOnError {
				break
			}
			continue
		}
		req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/activities", nil, activityCreateBody(item))
		if err != nil {
			return nil, err
		}
		var payload any
		if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
			failed++
			results = append(results, batchItemResult{Index: i, Error: wrapAPIError(err).Error()})
			if args.StopOnError {
				break
			}
			continue
		}
		_, m := extractItemData(payload)
		succeeded++
		results = append(results, batchItemResult{Index: i, OK: true, Activity: pipedrive.NormalizeActivity(m)})
	}
	if succeeded > 0 {
		invalidateActivitiesCache(client, 0)
	}
	return internal.Wrap(map[string]any{
		"results":   results,
		"succeeded": succeeded,
		"failed":    failed,
	}, nil), nil
}

var ActivitiesBatchCreate = mcppipedrive.MustTool(
	"pipedrive.activities.batch_create",
	"Create up to 100 activities in one call (write; sequential POSTs with per-item results). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	activitiesBatchCreate,
	mcp.WithTitleAnnotation("Batch-create activities"),
)
```

(b) Add to `pipedrive/normalize.go` (after the `NormalizedFilter` section, line 359):

```go
// NormalizedWebhook is the stable shape for a Pipedrive webhook subscription.
// HTTP auth credentials are intentionally omitted.
type NormalizedWebhook struct {
	ID              int64  `json:"id"`
	Name            string `json:"name,omitempty"`
	SubscriptionURL string `json:"subscription_url"`
	EventAction     string `json:"event_action"`
	EventObject     string `json:"event_object"`
	UserID          int64  `json:"user_id,omitempty"`
	IsActive        bool   `json:"is_active"`
	Version         string `json:"version,omitempty"`
	AddTime         string `json:"add_time,omitempty"`
}

func NormalizeWebhook(raw map[string]any) NormalizedWebhook {
	return NormalizedWebhook{
		ID:              toInt64(raw["id"]),
		Name:            toString(raw["name"]),
		SubscriptionURL: toString(raw["subscription_url"]),
		EventAction:     toString(raw["event_action"]),
		EventObject:     toString(raw["event_object"]),
		UserID:          relatedID(raw, "user_id"),
		IsActive:        toBool(firstNonNil(raw, "is_active", "active_flag")),
		Version:         toString(raw["version"]),
		AddTime:         toString(raw["add_time"]),
	}
}

func NormalizeWebhookList(raw []any) []NormalizedWebhook {
	out := make([]NormalizedWebhook, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeWebhook(m))
		}
	}
	return out
}
```

(c) Create `tools/webhooks.go`:

```go
package tools

// Webhook subscription management (v1 — v2 has no webhooks surface).
// The MCP only manages subscriptions; receiving the callbacks requires a
// publicly reachable HTTPS endpoint (e.g. an n8n Webhook trigger URL).

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type WebhooksListParams struct {
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type WebhooksCreateParams struct {
	Name             string `json:"name" jsonschema:"description=Webhook name (max 255 chars)"`
	SubscriptionURL  string `json:"subscription_url" jsonschema:"description=Public HTTPS endpoint Pipedrive will POST events to"`
	EventAction      string `json:"event_action" jsonschema:"description=Event action: create|change|delete|*"`
	EventObject      string `json:"event_object" jsonschema:"description=Event object: activity|deal|lead|note|organization|person|pipeline|product|stage|user|*"`
	UserID           int64  `json:"user_id,omitempty" jsonschema:"description=Only send events visible to this user ID (default: creator)"`
	HTTPAuthUser     string `json:"http_auth_user,omitempty" jsonschema:"description=HTTP basic auth username for the receiver"`
	HTTPAuthPassword string `json:"http_auth_password,omitempty" jsonschema:"description=HTTP basic auth password for the receiver"`
	Version          string `json:"version,omitempty" jsonschema:"description=Webhook payload version: 1.0|2.0 (default 2.0)"`
}

type WebhooksDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Webhook ID to delete (discover via pipedrive.webhooks.list)"`
}

func webhooksList(ctx context.Context, args WebhooksListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.list", guardRead); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	mode, err := pipedrive.ValidCacheMode(args.CacheMode)
	if err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/webhooks", nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/webhooks", nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Metadata, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	data := map[string]any{"webhooks": pipedrive.NormalizeWebhookList(arr)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func webhooksCreate(ctx context.Context, args WebhooksCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Name, "name"); err != nil {
		return nil, err
	}
	if err := internal.RequireID(args.SubscriptionURL, "subscription_url"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.EventAction, "event_action", "create", "change", "delete", "*"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.EventObject, "event_object",
		"activity", "deal", "lead", "note", "organization", "person", "pipeline", "product", "stage", "user", "*"); err != nil {
		return nil, err
	}
	if args.Version != "" && args.Version != "1.0" && args.Version != "2.0" {
		return nil, fmt.Errorf("version must be 1.0 or 2.0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":             args.Name,
		"subscription_url": args.SubscriptionURL,
		"event_action":     args.EventAction,
		"event_object":     args.EventObject,
	}
	setIfNonZeroInt(body, "user_id", args.UserID)
	setIfNonZero(body, "http_auth_user", args.HTTPAuthUser)
	setIfNonZero(body, "http_auth_password", args.HTTPAuthPassword)
	setIfNonZero(body, "version", args.Version)

	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/webhooks", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/webhooks")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"webhook": pipedrive.NormalizeWebhook(m),
		"raw":     internal.MaskSensitive(raw),
	}, nil), nil
}

func webhooksDelete(ctx context.Context, args WebhooksDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.delete", guardDelete); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	path := "/webhooks/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V1, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/webhooks")
	return internal.Wrap(map[string]any{
		"deleted": true,
		"id":      args.ID,
		"raw":     internal.MaskSensitive(payload),
	}, nil), nil
}

var WebhooksList = mcppipedrive.MustTool("pipedrive.webhooks.list",
	"List webhook subscriptions (v1).",
	webhooksList,
	mcp.WithTitleAnnotation("List webhooks"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true))

var WebhooksCreate = mcppipedrive.MustTool("pipedrive.webhooks.create",
	"Create a webhook subscription (write; Pipedrive POSTs matching events to subscription_url). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	webhooksCreate,
	mcp.WithTitleAnnotation("Create webhook"))

var WebhooksDelete = mcppipedrive.MustTool("pipedrive.webhooks.delete",
	"Delete a webhook subscription (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	webhooksDelete,
	mcp.WithTitleAnnotation("Delete webhook"),
	mcp.WithDestructiveHintAnnotation(true))
```

(d) In `tools/registry.go`:
- extend the activities line: `ActivitiesList, ActivitiesGet, ActivitiesCreate, ActivitiesUpdate, ActivitiesDelete, ActivitiesBatchCreate,`
- add before the cache line:

```go
		// webhooks
		WebhooksList, WebhooksCreate, WebhooksDelete,
```

(e) Add `"pipedrive.activities.batch_create"`, `"pipedrive.webhooks.list"`, `"pipedrive.webhooks.create"`, `"pipedrive.webhooks.delete"` to the registry test's `must` list.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./tools/ ./pipedrive/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tools/activities.go tools/activities_batch.go tools/activities_batch_test.go tools/webhooks.go tools/registry.go tools/registry_test.go tools/list_params_query_test.go pipedrive/normalize.go
git commit -m "feat(throughput): activities.batch_create and webhook management tools

batch_create collapses ~500 per-run MCP round-trips into ~10 while
keeping per-item error visibility. Webhook tools manage subscriptions
(e.g. an n8n receiver for deal-created events) — receiving callbacks
stays external.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Polish — mail direction, compound delete-gating message, docs

**Files:**
- Modify: `pipedrive/normalize_mail.go` (`NormalizedMailMessage` + `NormalizeMailMessage`)
- Modify: `tools/common.go:51-63` (guardDelete messages)
- Modify: `README.md` (tool catalog, counts, gating text)
- Modify: `docs/filters.md` (create/update section)
- Modify: `docs/roles/admin.md` (add the 9 new tools)
- Test: `pipedrive/normalize_mail_test.go`, `tools/guardrails_test.go`

**Interfaces:**
- Consumes: existing `NormalizeMailMessage`, `disabledResult`.
- Produces: `NormalizedMailMessage.Direction string` (`"draft" | "outgoing" | "incoming"`).

**Verified API facts:** No mail object has a literal `direction` field. Message objects carry `sent_flag` (0/1 — whether the message was sent [by a workspace user]) and `draft_flag`. Direction is therefore derived, not surfaced verbatim.

- [ ] **Step 1: Write the failing tests**

Append to `pipedrive/normalize_mail_test.go`:

```go
func TestNormalizeMailMessage_Direction(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"sent → outgoing", map[string]any{"id": float64(1), "sent_flag": float64(1)}, "outgoing"},
		{"unsent → incoming", map[string]any{"id": float64(2), "sent_flag": float64(0)}, "incoming"},
		{"no flags → incoming", map[string]any{"id": float64(3)}, "incoming"},
		{"draft wins", map[string]any{"id": float64(4), "draft_flag": float64(1), "sent_flag": float64(1)}, "draft"},
	}
	for _, tc := range cases {
		if got := NormalizeMailMessage(tc.raw, BodyFormatNone).Direction; got != tc.want {
			t.Errorf("%s: Direction = %q, want %q", tc.name, got, tc.want)
		}
	}
}
```

Append to `tools/guardrails_test.go`:

```go
func TestEnsureToolAllowed_DeleteMessagesMentionBothFlags(t *testing.T) {
	// write off: message must say delete needs both flags, not just write.
	ctx := ctxWith(pipedrive.Config{AllowWrite: false, AllowDelete: true})
	res, _ := ensureToolAllowed(ctx, "pipedrive.deals.delete", guardDelete)
	d, ok := res.(disabledResult)
	if !ok {
		t.Fatalf("expected disabledResult, got %T", res)
	}
	if !strings.Contains(d.Message, "PIPEDRIVE_ALLOW_WRITE") || !strings.Contains(d.Message, "PIPEDRIVE_ALLOW_DELETE") {
		t.Errorf("write-off delete message must name BOTH flags: %q", d.Message)
	}

	ctx = ctxWith(pipedrive.Config{AllowWrite: true, AllowDelete: false})
	res, _ = ensureToolAllowed(ctx, "pipedrive.deals.delete", guardDelete)
	d, ok = res.(disabledResult)
	if !ok {
		t.Fatalf("expected disabledResult, got %T", res)
	}
	if !strings.Contains(d.Message, "PIPEDRIVE_ALLOW_WRITE") || !strings.Contains(d.Message, "PIPEDRIVE_ALLOW_DELETE") {
		t.Errorf("delete-off message must name BOTH flags: %q", d.Message)
	}
}
```

(add `"strings"` to that file's imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pipedrive/ -run Direction -v && go test ./tools/ -run BothFlags -v`
Expected: FAIL — `Direction` undefined (compile) and/or message assertions failing.

- [ ] **Step 3: Implement**

(a) In `pipedrive/normalize_mail.go`, add to `NormalizedMailMessage` (after `MessageTime`, line 57):

```go
	// Direction is derived: draft_flag → "draft"; sent_flag → "outgoing"
	// (sent by a workspace user); otherwise "incoming". The raw API has no
	// direction field.
	Direction      string                      `json:"direction,omitempty"`
```

In `NormalizeMailMessage`, after the struct literal (line 130), add:

```go
	switch {
	case toInt64(raw["draft_flag"]) != 0:
		m.Direction = "draft"
	case toInt64(raw["sent_flag"]) != 0:
		m.Direction = "outgoing"
	default:
		m.Direction = "incoming"
	}
```

(b) In `tools/common.go`, replace the `guardDelete` case (lines 51-63) with:

```go
	case guardDelete:
		if !cfg.AllowWrite {
			return disabledResult{
				Error:   "write_disabled",
				Message: "Delete tools require BOTH PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true; write is currently disabled",
			}, nil
		}
		if !cfg.AllowDelete {
			return disabledResult{
				Error:   "delete_disabled",
				Message: "Delete tools require BOTH PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true; delete is currently disabled",
			}, nil
		}
```

(c) `README.md` updates:
- Line 69: `## Tools (59 total)` → `## Tools (68 total)`
- Read section: add after the `pipedrive.filters.list` line:
  - `- `pipedrive.webhooks.list` — webhook subscriptions`
- Read section: update the `context.get` line to `- `pipedrive.context.get` — users, pipelines, stages, deal-fields, activity-types (cached)`
- Write section: add:
  - `- `pipedrive.deals.{archive,unarchive}` — sets `is_archived` via PATCH`
  - `- `pipedrive.deal_fields.add_option` — append options to enum/set deal fields`
  - `- `pipedrive.filters.{create,update}` — saved filters for server-side (incl. custom-field) filtering`
  - `- `pipedrive.activities.batch_create` — up to 100 activities per call`
  - `- `pipedrive.webhooks.create``
- Delete section: add `- `pipedrive.webhooks.delete``
- After the delete-section heading `### Delete (gated by ...)`, add the sentence: `Delete tools require **both** flags — `PIPEDRIVE_ALLOW_DELETE=true` alone is not sufficient.`
- Role table: change the admin row's tool count from 54 to 63, and add below the table: `Token figures were measured against the pre-68-tool catalog; treat them as lower bounds until re-measured.`

(d) `docs/roles/admin.md`: append the 9 new tool names (`pipedrive.deals.archive`, `pipedrive.deals.unarchive`, `pipedrive.deal_fields.add_option`, `pipedrive.filters.create`, `pipedrive.filters.update`, `pipedrive.activities.batch_create`, `pipedrive.webhooks.list`, `pipedrive.webhooks.create`, `pipedrive.webhooks.delete`) to its `PIPEDRIVE_ALLOWED_TOOLS` recipe, matching the file's existing format. Other role recipes are unchanged by design.

(e) `docs/filters.md`: append a section:

```markdown
## Creating and updating filters

`pipedrive.filters.create` (write-gated) creates a saved filter; `pipedrive.filters.update` replaces its `conditions` (and optionally `name`). The filter `type` is immutable after creation.

Conditions are Pipedrive's two-level glue tree (max 16 leaf conditions). Example — deals where the custom field `mary_processed_at` is set AND `mary_archived_at` is empty:

    {
      "glue": "and",
      "conditions": [
        {
          "glue": "and",
          "conditions": [
            {"object": "deal", "field_id": "<id of mary_processed_at>", "operator": "IS NOT NULL", "value": null},
            {"object": "deal", "field_id": "<id of mary_archived_at>", "operator": "IS NULL", "value": null}
          ]
        },
        {"glue": "or", "conditions": []}
      ]
    }

Resolve `field_id` values (numeric field IDs, not hash keys) from `pipedrive.context.get` → `deal_fields`. Then pass the returned filter `id` as `filter_id` to `pipedrive.deals.list` for true server-side filtering.
```

- [ ] **Step 4: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 5: Commit**

```bash
git add pipedrive/normalize_mail.go pipedrive/normalize_mail_test.go tools/common.go tools/guardrails_test.go README.md docs/filters.md docs/roles/admin.md
git commit -m "feat(polish): mail direction field, compound delete-gating message, docs for 68-tool catalog

direction (draft|outgoing|incoming) is derived from draft_flag/sent_flag
so reply detection stops string-matching the lead's address. Delete
gating errors now spell out that BOTH flags are required.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final verification (after all tasks)

- [ ] `go build ./... && go test ./...` — all green.
- [ ] `go vet ./...` — clean.
- [ ] Spot-check the registered surface: `go test ./tools/ -run TestRegistry -v` — the 9 new names present.
- [ ] Cross-check README count (68) against `len(allTools())`.
- [ ] Push branch and open PR to `main` (title: "Close mary TODO.md Pipedrive MCP gaps: 9 new tools, archived/label visibility, docs"); run the code-review skill on the diff first.
