package tools

// Notes are served via Pipedrive API v1 (no v2 equivalent at the time of
// writing). Keep the split hidden behind the stable pipedrive.notes.* tools.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type NotesListParams struct {
	Limit          int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Start          int    `json:"start,omitempty" jsonschema:"description=Offset for pagination (v1 uses start)"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=Filter by deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=Filter by person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Filter by organization ID"`
	UserID         int64  `json:"user_id,omitempty" jsonschema:"description=Filter by user ID"`
	FilterID       int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	StartDate      string `json:"start_date,omitempty" jsonschema:"description=YYYY-MM-DD lower bound on add_time"`
	EndDate        string `json:"end_date,omitempty" jsonschema:"description=YYYY-MM-DD upper bound on add_time"`
	IncludeRaw     bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload"`
	CacheMode      string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type NotesCreateParams struct {
	Content        string `json:"content" jsonschema:"description=Note content (required)"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=Linked deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=Linked person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Linked organization ID"`
	LeadID         string `json:"lead_id,omitempty" jsonschema:"description=Linked lead UUID"`
	UserID         int64  `json:"user_id,omitempty" jsonschema:"description=Author user ID (e.g. the deal owner). Defaults to the token user; only admin tokens can set another author — check user_id in the response"`
	PinnedToDeal   bool   `json:"pinned_to_deal,omitempty" jsonschema:"description=Pin note to the linked deal"`
	PinnedToPerson bool   `json:"pinned_to_person,omitempty" jsonschema:"description=Pin note to the linked person"`
	PinnedToOrg    bool   `json:"pinned_to_organization,omitempty" jsonschema:"description=Pin note to the linked organization"`
}

type NotesUpdateParams struct {
	ID             int64  `json:"id" jsonschema:"description=Note ID to update"`
	Content        string `json:"content,omitempty" jsonschema:"description=New note content"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=New linked deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=New linked person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=New linked organization ID"`
	LeadID         string `json:"lead_id,omitempty" jsonschema:"description=New linked lead UUID"`
	UserID         int64  `json:"user_id,omitempty" jsonschema:"description=New author user ID (e.g. the deal owner). Only admin tokens can change the author — check user_id in the response"`
	PinnedToDeal   *bool  `json:"pinned_to_deal,omitempty" jsonschema:"description=Set pinned-to-deal true|false"`
	PinnedToPerson *bool  `json:"pinned_to_person,omitempty" jsonschema:"description=Set pinned-to-person true|false"`
	PinnedToOrg    *bool  `json:"pinned_to_organization,omitempty" jsonschema:"description=Set pinned-to-organization true|false"`
}

func notesList(ctx context.Context, args NotesListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.notes.list", guardRead); err != nil {
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
	q, err := internal.AddV1Pagination(url.Values{}, args.Start, args.Limit)
	if err != nil {
		return nil, err
	}
	if args.DealID != 0 {
		q.Set("deal_id", strconv.FormatInt(args.DealID, 10))
	}
	if args.PersonID != 0 {
		q.Set("person_id", strconv.FormatInt(args.PersonID, 10))
	}
	if args.OrganizationID != 0 {
		q.Set("org_id", strconv.FormatInt(args.OrganizationID, 10))
	}
	if args.UserID != 0 {
		q.Set("user_id", strconv.FormatInt(args.UserID, 10))
	}
	if args.FilterID != 0 {
		q.Set("filter_id", strconv.FormatInt(args.FilterID, 10))
	}
	if args.StartDate != "" {
		q.Set("start_date", args.StartDate)
	}
	if args.EndDate != "" {
		q.Set("end_date", args.EndDate)
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/notes", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/notes", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.List, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	data := map[string]any{"notes": pipedrive.NormalizeNoteList(arr)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "start": args.Start, "cache": cacheMeta}
	return internal.Wrap(data, meta), nil
}

func notesCreate(ctx context.Context, args NotesCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.notes.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Content, "content"); err != nil {
		return nil, err
	}
	if args.DealID == 0 && args.PersonID == 0 && args.OrganizationID == 0 && args.LeadID == "" {
		return nil, fmt.Errorf("at least one of deal_id, person_id, organization_id or lead_id is required")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"content": args.Content}
	setIfNonZeroInt(body, "deal_id", args.DealID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZero(body, "lead_id", args.LeadID)
	setIfNonZeroInt(body, "user_id", args.UserID)
	if args.PinnedToDeal {
		body["pinned_to_deal_flag"] = 1
	}
	if args.PinnedToPerson {
		body["pinned_to_person_flag"] = 1
	}
	if args.PinnedToOrg {
		body["pinned_to_organization_flag"] = 1
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/notes", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/notes")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{"note": pipedrive.NormalizeNote(m), "raw": internal.MaskSensitive(raw)}, nil), nil
}

func notesUpdate(ctx context.Context, args NotesUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.notes.update", guardWrite); err != nil {
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
	body := map[string]any{}
	setIfNonZero(body, "content", args.Content)
	setIfNonZeroInt(body, "deal_id", args.DealID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZero(body, "lead_id", args.LeadID)
	setIfNonZeroInt(body, "user_id", args.UserID)
	setPinnedFlag(body, "pinned_to_deal_flag", args.PinnedToDeal)
	setPinnedFlag(body, "pinned_to_person_flag", args.PinnedToPerson)
	setPinnedFlag(body, "pinned_to_organization_flag", args.PinnedToOrg)
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/notes/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V1, http.MethodPut, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/notes")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{"note": pipedrive.NormalizeNote(m), "raw": internal.MaskSensitive(raw)}, nil), nil
}

// setPinnedFlag maps an optional bool to Pipedrive's v1 0/1 pinned flag, so
// callers can both pin and unpin.
func setPinnedFlag(body map[string]any, key string, v *bool) {
	if v == nil {
		return
	}
	if *v {
		body[key] = 1
	} else {
		body[key] = 0
	}
}

var NotesList = mcppipedrive.MustTool("pipedrive.notes.list",
	"List notes (Pipedrive API v1) filtered by filter_id / deal / person / organization / user / start_date-end_date.",
	notesList,
	mcp.WithTitleAnnotation("List notes"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var NotesCreate = mcppipedrive.MustTool("pipedrive.notes.create",
	"Create a note attached to a deal/person/organization/lead (write). Pass user_id to set the author (e.g. the deal owner). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	notesCreate,
	mcp.WithTitleAnnotation("Create note"))

var NotesUpdate = mcppipedrive.MustTool("pipedrive.notes.update",
	"Update a note's content, links, pinning or author (write; v1 PUT, only the fields you pass change). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	notesUpdate,
	mcp.WithTitleAnnotation("Update note"))

