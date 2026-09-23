package pipedrive

import (
	"strings"
	"testing"
)

func TestNormalizeMailThreadCompactDropsParties(t *testing.T) {
	raw := map[string]any{
		"id":                     float64(92808),
		"subject":                "Re: hi",
		"snippet":                strings.Repeat("x", 240),
		"read_flag":              float64(0),
		"has_attachments_flag":   float64(1),
		"deal_id":                float64(84369),
		"last_message_timestamp": "2026-05-14T14:03:14.000Z",
		"parties": map[string]any{
			"from": []any{map[string]any{"name": "X", "email_address": "x@example.com"}},
			"to":   []any{map[string]any{"email_address": "y@example.com"}},
		},
	}
	got := NormalizeMailThread(raw, MailFieldsCompact)
	if got.ID != 92808 {
		t.Fatalf("ID: got %d, want 92808", got.ID)
	}
	if got.Parties != nil {
		t.Fatalf("compact projection must not include parties tree: %+v", got.Parties)
	}
	if got.From == nil || got.From.Email != "x@example.com" {
		t.Fatalf("compact projection must include primary from: %+v", got.From)
	}
	if !strings.HasSuffix(got.Snippet, "…") || len(got.Snippet) > MailCompactSnippetMax+4 {
		t.Fatalf("snippet not truncated: %q (len=%d)", got.Snippet, len(got.Snippet))
	}
	if !got.HasAttachments {
		t.Fatal("expected has_attachments_flag=1 to surface as true")
	}
	if got.DealID == nil || *got.DealID != 84369 {
		t.Fatalf("expected deal_id 84369, got %+v", got.DealID)
	}
}

func TestNormalizeMailThreadFullKeepsParties(t *testing.T) {
	raw := map[string]any{
		"id":      float64(1),
		"subject": "hi",
		"snippet": "short",
		"parties": map[string]any{
			"from": []any{map[string]any{"email_address": "a@x.com"}},
			"to":   []any{map[string]any{"email_address": "b@x.com"}},
			"cc":   []any{map[string]any{"email_address": "c@x.com"}},
		},
	}
	got := NormalizeMailThread(raw, MailFieldsFull)
	if got.Parties == nil {
		t.Fatal("full projection must include parties tree")
	}
	if len(got.Parties.From) != 1 || got.Parties.From[0].Email != "a@x.com" {
		t.Fatalf("from: %+v", got.Parties.From)
	}
	if len(got.Parties.Cc) != 1 || got.Parties.Cc[0].Email != "c@x.com" {
		t.Fatalf("cc: %+v", got.Parties.Cc)
	}
	if got.From != nil {
		t.Fatalf("full projection must NOT include compact From: %+v", got.From)
	}
}

func TestNormalizeMailMessageBodyFormats(t *testing.T) {
	raw := map[string]any{
		"id":             float64(128140),
		"mail_thread_id": float64(92808),
		"subject":        "hi",
		"body":           `<p>Hello <b>world</b></p>`,
		"message_time":   "2026-05-14T14:03:14.000Z",
		"read_flag":      float64(1),
	}

	// Default → text. We don't pin exact bytes since html2text formatting
	// can shift across library versions; assert markup is gone and prose
	// survives.
	got := NormalizeMailMessage(raw, BodyFormatText)
	if got.BodyFormat != BodyFormatText {
		t.Fatalf("expected text format, got %q", got.BodyFormat)
	}
	if strings.Contains(got.Body, "<") || strings.Contains(got.Body, ">") {
		t.Fatalf("text body still contains markup: %q", got.Body)
	}
	if !strings.Contains(got.Body, "Hello") || !strings.Contains(got.Body, "world") {
		t.Fatalf("text body lost prose: %q", got.Body)
	}

	// html → raw passthrough
	got = NormalizeMailMessage(raw, BodyFormatHTML)
	if got.BodyFormat != BodyFormatHTML || got.Body != raw["body"].(string) {
		t.Fatalf("html mode should pass raw through, got format=%q body=%q", got.BodyFormat, got.Body)
	}

	// none → no body, no format hint
	got = NormalizeMailMessage(raw, BodyFormatNone)
	if got.Body != "" || got.BodyFormat != "" {
		t.Fatalf("none mode should omit body: %+v", got)
	}

	// message_time field name is `message_time`, not `email_message_timestamp`
	if got.MessageTime != "2026-05-14T14:03:14.000Z" {
		t.Fatalf("expected message_time carried through, got %q", got.MessageTime)
	}
	if got.ReadFlag != 1 {
		t.Fatalf("expected read_flag=1 mutable field carried, got %d", got.ReadFlag)
	}
}

func TestNormalizeBodyFormatCoercion(t *testing.T) {
	if NormalizeBodyFormat("") != BodyFormatText {
		t.Fatal("empty should default to text")
	}
	if NormalizeBodyFormat("garbage") != BodyFormatText {
		t.Fatal("unknown should fall back to text")
	}
	if NormalizeBodyFormat(BodyFormatHTML) != BodyFormatHTML {
		t.Fatal("html should pass through")
	}
	if NormalizeBodyFormat(BodyFormatNone) != BodyFormatNone {
		t.Fatal("none should pass through")
	}
}

func TestNormalizeMailFieldsCoercion(t *testing.T) {
	if NormalizeMailFields("") != MailFieldsCompact {
		t.Fatal("empty should default to compact")
	}
	if NormalizeMailFields("garbage") != MailFieldsCompact {
		t.Fatal("unknown should fall back to compact")
	}
	if NormalizeMailFields(MailFieldsFull) != MailFieldsFull {
		t.Fatal("full should pass through")
	}
}

func TestHTMLToTextStripsMarkupAndPreservesLinks(t *testing.T) {
	in := `<p>Open <a href="https://example.com/x">here</a> please.</p>`
	out := HTMLToText(in)
	if strings.Contains(out, "<") || strings.Contains(out, ">") {
		t.Fatalf("expected no angle brackets, got %q", out)
	}
	if !strings.Contains(out, "https://example.com/x") {
		t.Fatalf("expected link URL preserved, got %q", out)
	}
}

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
