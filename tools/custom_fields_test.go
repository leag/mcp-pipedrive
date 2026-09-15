package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mcp-pipedrive/pipedrive"
)

const testDealFields = `{"success":true,"data":[
  {"key":"title","name":"Title","field_type":"varchar"},
  {"key":"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111","name":"# of devices","field_type":"double"},
  {"key":"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222","name":"Prey plan","field_type":"enum",
   "options":[{"id":51,"label":"tracking"},{"id":53,"label":"full_suite"}]}
]}`

// fieldsTransport serves real custom-field metadata plus a canned entity
// payload, so handlers can be exercised through the full decode path.
type fieldsTransport struct {
	entity   string
	lastBody []byte
}

func (f *fieldsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/dealFields") {
		return jsonResponse(req, testDealFields), nil
	}
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		f.lastBody = b
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	return jsonResponse(req, f.entity), nil
}

func newFieldsCtx(t *testing.T, entity string, write bool) (context.Context, *fieldsTransport) {
	t.Helper()
	transport := &fieldsTransport{entity: entity}
	client := &pipedrive.Client{
		BaseURL:  "https://test.pipedrive.com",
		HTTP:     &http.Client{Transport: transport},
		APIToken: "test-token",
		AuthMode: pipedrive.AuthModeToken,
	}
	ctx := pipedrive.WithConfig(context.Background(), pipedrive.Config{AllowWrite: write})
	return pipedrive.WithClient(ctx, client), transport
}

func TestDealsList_DecodesCustomFields(t *testing.T) {
	payload := `{"success":true,"data":[{"id":1,"title":"D","custom_fields":{
      "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111":11,
      "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222":53,
      "cccc3333cccc3333cccc3333cccc3333cccc3333":null}}]}`
	ctx, _ := newFieldsCtx(t, payload, false)

	res, err := dealsList(ctx, DealsListParams{})
	if err != nil {
		t.Fatalf("dealsList: %v", err)
	}
	deals := res.(map[string]any)["data"].(map[string]any)["deals"].([]pipedrive.NormalizedDeal)
	if len(deals) != 1 {
		t.Fatalf("want 1 deal, got %d", len(deals))
	}
	cf := deals[0].CustomFields
	if got := cf["# of devices"]; got != float64(11) {
		t.Errorf("# of devices = %#v, want 11", got)
	}
	if got := cf["Prey plan"]; got != "full_suite" {
		t.Errorf("Prey plan = %#v, want full_suite", got)
	}
	if len(cf) != 2 {
		t.Errorf("null custom field not dropped: %#v", cf)
	}
}

func TestDealsUpdate_EncodesNamesAndLabels(t *testing.T) {
	ctx, transport := newFieldsCtx(t, `{"success":true,"data":{"id":5}}`, true)

	_, err := dealsUpdate(ctx, DealsUpdateParams{
		ID:           5,
		CustomFields: map[string]any{"Prey plan": "full_suite", "# of devices": 11},
	})
	if err != nil {
		t.Fatalf("dealsUpdate: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(transport.lastBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	cf, ok := body["custom_fields"].(map[string]any)
	if !ok {
		t.Fatalf("custom_fields missing from body: %s", transport.lastBody)
	}
	if got := cf["bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"]; got != float64(53) {
		t.Errorf("enum label not encoded to id: %#v", got)
	}
	if got := cf["aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"]; got != float64(11) {
		t.Errorf("field name not encoded to hash: %#v", got)
	}
}

// A name we cannot resolve must fail loudly: Pipedrive would accept the write
// and silently ignore the field, reporting success while dropping data.
func TestDealsUpdate_RejectsUnknownFieldName(t *testing.T) {
	ctx, _ := newFieldsCtx(t, `{"success":true,"data":{"id":5}}`, true)

	_, err := dealsUpdate(ctx, DealsUpdateParams{
		ID:           5,
		CustomFields: map[string]any{"Nonexistent field": 1},
	})
	if err == nil {
		t.Fatal("want error for unresolvable custom field name")
	}
	if !strings.Contains(err.Error(), "Nonexistent field") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}
