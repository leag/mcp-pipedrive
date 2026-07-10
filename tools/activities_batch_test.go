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
