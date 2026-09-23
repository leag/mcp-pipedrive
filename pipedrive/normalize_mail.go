package pipedrive

import "strings"

// NormalizedMailParticipant projects a Pipedrive mail participant
// (a {name, email_address, linked_person_id, linked_person_name} entry)
// to the minimum agent-useful shape. The linked_person_* fields are
// dropped — they bloat list responses and are rarely consulted.
type NormalizedMailParticipant struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// NormalizedMailParties is the full participant tree on a thread under
// MailFieldsFull projection. Compact projection uses a single primary
// `from` instead.
type NormalizedMailParties struct {
	From []NormalizedMailParticipant `json:"from,omitempty"`
	To   []NormalizedMailParticipant `json:"to,omitempty"`
	Cc   []NormalizedMailParticipant `json:"cc,omitempty"`
	Bcc  []NormalizedMailParticipant `json:"bcc,omitempty"`
}

// NormalizedMailThread is the agent-facing shape for /mailbox/mailThreads.
// Pipedrive bookkeeping fields (account_id, assigned_user_ids,
// mail_link_tracking_*, external_*_flag, nylas_id, s3_bucket*, etc.) are
// dropped at the boundary so they never burn LLM context.
type NormalizedMailThread struct {
	ID                   int64                      `json:"id"`
	Subject              string                     `json:"subject,omitempty"`
	Snippet              string                     `json:"snippet,omitempty"`
	ReadFlag             int                        `json:"read_flag"`
	ArchivedFlag         int                        `json:"archived_flag,omitempty"`
	DealID               *int64                     `json:"deal_id,omitempty"`
	PersonID             *int64                     `json:"person_id,omitempty"`
	MessageCount         int                        `json:"message_count,omitempty"`
	LastMessageTimestamp string                     `json:"last_message_timestamp,omitempty"`
	HasAttachments       bool                       `json:"has_attachments,omitempty"`
	// From is populated under MailFieldsCompact (primary sender only).
	From *NormalizedMailParticipant `json:"from,omitempty"`
	// Parties is populated under MailFieldsFull (entire from/to/cc/bcc tree).
	Parties *NormalizedMailParties `json:"parties,omitempty"`
}

// NormalizedMailMessage is the agent-facing shape for individual messages
// (both /mailbox/mailMessages/{id} and inner items from
// /deals/{id}/mailMessages after envelope unwrap).
type NormalizedMailMessage struct {
	ID             int64                       `json:"id"`
	MailThreadID   int64                       `json:"mail_thread_id,omitempty"`
	Subject        string                      `json:"subject,omitempty"`
	Snippet        string                      `json:"snippet,omitempty"`
	From           []NormalizedMailParticipant `json:"from,omitempty"`
	To             []NormalizedMailParticipant `json:"to,omitempty"`
	Cc             []NormalizedMailParticipant `json:"cc,omitempty"`
	Bcc            []NormalizedMailParticipant `json:"bcc,omitempty"`
	MessageTime    string                      `json:"message_time,omitempty"`
	Direction      string                      `json:"direction,omitempty"`
	ReadFlag       int                         `json:"read_flag"`
	DraftFlag      int                         `json:"draft_flag,omitempty"`
	SentFlag       int                         `json:"sent_flag,omitempty"`
	DeletedFlag    int                         `json:"deleted_flag,omitempty"`
	HasAttachments bool                        `json:"has_attachments,omitempty"`
	Body           string                      `json:"body,omitempty"`
	BodyFormat     string                      `json:"body_format,omitempty"`
}

// NormalizeMailThread builds NormalizedMailThread from a raw thread map.
// `fields` selects compact vs full projection (see MailFieldsCompact /
// MailFieldsFull). Unknown values fall back to compact.
func NormalizeMailThread(raw map[string]any, fields string) NormalizedMailThread {
	t := NormalizedMailThread{
		ID:                   toInt64(raw["id"]),
		Subject:              toString(raw["subject"]),
		ReadFlag:             int(toInt64(raw["read_flag"])),
		ArchivedFlag:         int(toInt64(raw["archived_flag"])),
		MessageCount:         int(toInt64(raw["message_count"])),
		LastMessageTimestamp: toString(raw["last_message_timestamp"]),
		HasAttachments:       toInt64(raw["has_attachments_flag"]) != 0,
	}
	if id := relatedID(raw, "deal_id"); id != 0 {
		t.DealID = &id
	}
	if id := relatedID(raw, "person_id"); id != 0 {
		t.PersonID = &id
	}

	snippet := toString(raw["snippet"])
	parties, _ := raw["parties"].(map[string]any)

	if fields == MailFieldsFull {
		t.Snippet = snippet
		t.Parties = mailParties(parties)
	} else {
		t.Snippet = truncateSnippet(snippet, MailCompactSnippetMax)
		t.From = primaryFrom(parties)
	}
	return t
}

// NormalizeMailThreadList walks a raw thread array.
func NormalizeMailThreadList(raw []any, fields string) []NormalizedMailThread {
	out := make([]NormalizedMailThread, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, NormalizeMailThread(m, fields))
		}
	}
	return out
}

// NormalizeMailMessage builds NormalizedMailMessage from a raw message
// map. `bodyFormat` controls how the body is presented (text default,
// html raw, none omits).
func NormalizeMailMessage(raw map[string]any, bodyFormat string) NormalizedMailMessage {
	m := NormalizedMailMessage{
		ID:             toInt64(raw["id"]),
		MailThreadID:   toInt64(raw["mail_thread_id"]),
		Subject:        toString(raw["subject"]),
		Snippet:        toString(raw["snippet"]),
		From:           mailParticipants(raw["from"]),
		To:             mailParticipants(raw["to"]),
		Cc:             mailParticipants(raw["cc"]),
		Bcc:            mailParticipants(raw["bcc"]),
		MessageTime:    toString(raw["message_time"]),
		ReadFlag:       int(toInt64(raw["read_flag"])),
		DraftFlag:      int(toInt64(raw["draft_flag"])),
		SentFlag:       int(toInt64(raw["sent_flag"])),
		DeletedFlag:    int(toInt64(raw["deleted_flag"])),
		HasAttachments: toInt64(raw["has_attachments_flag"]) != 0,
	}
	// Direction is derived: draft_flag → "draft"; sent_flag → "outgoing"
	// (sent by a workspace user); otherwise "incoming". The raw API has no
	// direction field.
	switch {
	case toInt64(raw["draft_flag"]) != 0:
		m.Direction = "draft"
	case toInt64(raw["sent_flag"]) != 0:
		m.Direction = "outgoing"
	default:
		m.Direction = "incoming"
	}
	if bodyFormat == BodyFormatNone {
		return m
	}
	body := toString(raw["body"])
	if body == "" {
		body = toString(raw["body_content"])
	}
	if body == "" {
		return m
	}
	switch bodyFormat {
	case BodyFormatHTML:
		m.Body = body
		m.BodyFormat = BodyFormatHTML
	default:
		m.Body = HTMLToText(body)
		m.BodyFormat = BodyFormatText
	}
	return m
}

// NormalizeMailMessageList walks a raw message array (already unwrapped
// from any per-item envelope).
func NormalizeMailMessageList(raw []any, bodyFormat string) []NormalizedMailMessage {
	out := make([]NormalizedMailMessage, 0, len(raw))
	for _, v := range raw {
		if mm, ok := v.(map[string]any); ok {
			out = append(out, NormalizeMailMessage(mm, bodyFormat))
		}
	}
	return out
}

// NormalizeBodyFormat coerces a user-supplied body_format value to one
// of the BodyFormat* constants. Empty / unknown values become text.
func NormalizeBodyFormat(in string) string {
	switch in {
	case BodyFormatHTML, BodyFormatNone:
		return in
	default:
		return BodyFormatText
	}
}

// NormalizeMailFields coerces a user-supplied fields value to one of the
// MailFields* constants. Unknown values become compact.
func NormalizeMailFields(in string) string {
	if in == MailFieldsFull {
		return MailFieldsFull
	}
	return MailFieldsCompact
}

func mailParticipants(v any) []NormalizedMailParticipant {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]NormalizedMailParticipant, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		email := toString(m["email_address"])
		name := toString(m["name"])
		if email == "" && name == "" {
			continue
		}
		out = append(out, NormalizedMailParticipant{Name: name, Email: email})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mailParties(in map[string]any) *NormalizedMailParties {
	if len(in) == 0 {
		return nil
	}
	p := &NormalizedMailParties{
		From: mailParticipants(in["from"]),
		To:   mailParticipants(in["to"]),
		Cc:   mailParticipants(in["cc"]),
		Bcc:  mailParticipants(in["bcc"]),
	}
	if p.From == nil && p.To == nil && p.Cc == nil && p.Bcc == nil {
		return nil
	}
	return p
}

func primaryFrom(in map[string]any) *NormalizedMailParticipant {
	froms, ok := in["from"].([]any)
	if !ok || len(froms) == 0 {
		return nil
	}
	m, ok := froms[0].(map[string]any)
	if !ok {
		return nil
	}
	email := toString(m["email_address"])
	name := toString(m["name"])
	if email == "" && name == "" {
		return nil
	}
	return &NormalizedMailParticipant{Name: name, Email: email}
}

func truncateSnippet(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && (s[cut]&0xC0) == 0x80 { // skip UTF-8 continuation bytes
		cut--
	}
	return strings.TrimRight(s[:cut], " \t\n") + "…"
}
