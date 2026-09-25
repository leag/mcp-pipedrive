package tools

import (
	"encoding/json"
	"testing"

	"mcp-pipedrive/pipedrive"
)

func TestNotesCreate_SendsUserIDAsAuthor(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":9,"content":"hola","deal_id":42,"user_id":7}}`,
	})
	res, err := notesCreate(ctx, NotesCreateParams{Content: "hola", DealID: 42, UserID: 7})
	if err != nil {
		t.Fatalf("notesCreate: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(transport.requests))
	}
	req := transport.requests[0]
	if req.Method != "POST" || req.URL.Path != "/api/v1/notes" {
		t.Errorf("request = %s %s, want POST /api/v1/notes", req.Method, req.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(transport.bodies[0], &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["user_id"] != float64(7) || body["deal_id"] != float64(42) {
		t.Errorf("body must carry user_id=7 and deal_id=42, got %v", body)
	}
	out, _ := json.Marshal(res)
	var parsed struct {
		Data struct {
			Note pipedrive.NormalizedNote `json:"note"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.Data.Note.UserID != 7 {
		t.Errorf("normalized note user_id = %d, want 7 (result: %s)", parsed.Data.Note.UserID, out)
	}
}

func TestNotesCreate_OmitsUserIDWhenUnset(t *testing.T) {
	ctx, transport := newScriptedCtx(t, nil)
	if _, err := notesCreate(ctx, NotesCreateParams{Content: "hola", DealID: 42}); err != nil {
		t.Fatalf("notesCreate: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(transport.bodies[0], &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if _, ok := body["user_id"]; ok {
		t.Errorf("user_id must be omitted when not set (Pipedrive defaults to the token user), got %v", body)
	}
}

func TestNotesUpdate_PutsOnlyPassedFields(t *testing.T) {
	ctx, transport := newScriptedCtx(t, []string{
		`{"success":true,"data":{"id":9,"content":"hola","deal_id":42,"user_id":7}}`,
	})
	unpin := false
	if _, err := notesUpdate(ctx, NotesUpdateParams{ID: 9, UserID: 7, PinnedToDeal: &unpin}); err != nil {
		t.Fatalf("notesUpdate: %v", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(transport.requests))
	}
	req := transport.requests[0]
	if req.Method != "PUT" || req.URL.Path != "/api/v1/notes/9" {
		t.Errorf("request = %s %s, want PUT /api/v1/notes/9", req.Method, req.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(transport.bodies[0], &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	want := map[string]any{"user_id": float64(7), "pinned_to_deal_flag": float64(0)}
	if len(body) != len(want) {
		t.Fatalf("body = %v, want exactly %v", body, want)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, body[k], v)
		}
	}
}

func TestNotesUpdate_RejectsMissingIDAndEmptyBody(t *testing.T) {
	ctx, transport := newScriptedCtx(t, nil)
	if _, err := notesUpdate(ctx, NotesUpdateParams{UserID: 7}); err == nil {
		t.Error("expected error for missing id")
	}
	if _, err := notesUpdate(ctx, NotesUpdateParams{ID: 9}); err == nil {
		t.Error("expected error when no fields are passed")
	}
	if len(transport.requests) != 0 {
		t.Fatalf("must not call Pipedrive on invalid input; requests = %d", len(transport.requests))
	}
}

func TestNotesUpdate_WriteDisabled(t *testing.T) {
	ctx := ctxWith(pipedrive.Config{AllowWrite: false})
	res, err := notesUpdate(ctx, NotesUpdateParams{ID: 9, UserID: 7})
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	d, ok := res.(disabledResult)
	if !ok || d.Error != "write_disabled" {
		t.Fatalf("expected write_disabled, got %+v", res)
	}
}
