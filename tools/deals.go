package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

type DealsListParams struct {
	Limit        int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"description=Cursor from a previous page"`
	Status       string `json:"status,omitempty" jsonschema:"description=Filter by status: open|won|lost|deleted|all_not_deleted"`
	OwnerID      int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by deal owner user ID"`
	PipelineID   int64  `json:"pipeline_id,omitempty" jsonschema:"description=Filter by pipeline ID"`
	StageID      int64  `json:"stage_id,omitempty" jsonschema:"description=Filter by stage ID"`
	FilterID     int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	IsArchived   *bool  `json:"is_archived,omitempty" jsonschema:"description=true → list archived deals (targets GET /deals/archived); false or omitted → non-archived deals"`
	UpdatedSince string `json:"updated_since,omitempty" jsonschema:"description=RFC3339 lower bound on update_time (e.g. 2026-05-01T00:00:00Z)"`
	UpdatedUntil string `json:"updated_until,omitempty" jsonschema:"description=RFC3339 upper bound on update_time"`
	IncludeRaw   bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode    string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type DealsGetParams struct {
	ID         int64  `json:"id" jsonschema:"description=Deal ID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type DealsSearchParams struct {
	Term      string `json:"term" jsonschema:"description=Search term (minimum 2 characters)"`
	Fields    string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields to search: custom_fields|notes|title (default title)"`
	Status    string `json:"status,omitempty" jsonschema:"description=Filter by status: open|won|lost"`
	Exact     bool   `json:"exact_match,omitempty" jsonschema:"description=Require exact match"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	CacheMode string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type DealsCreateParams struct {
	Title          string         `json:"title" jsonschema:"description=Deal title"`
	Value          float64        `json:"value,omitempty" jsonschema:"description=Deal monetary value"`
	Currency       string         `json:"currency,omitempty" jsonschema:"description=Currency code (e.g. USD EUR)"`
	OwnerID        int64          `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	PersonID       int64          `json:"person_id,omitempty" jsonschema:"description=Linked person ID"`
	OrganizationID int64          `json:"organization_id,omitempty" jsonschema:"description=Linked organization ID"`
	PipelineID     int64          `json:"pipeline_id,omitempty" jsonschema:"description=Pipeline ID"`
	StageID        int64          `json:"stage_id,omitempty" jsonschema:"description=Stage ID"`
	Status         string         `json:"status,omitempty" jsonschema:"description=Status: open|won|lost"`
	CustomFields   map[string]any `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs"`
}

type DealsUpdateParams struct {
	ID             int64          `json:"id" jsonschema:"description=Deal ID to update"`
	Title          string         `json:"title,omitempty" jsonschema:"description=New deal title"`
	Value          *float64       `json:"value,omitempty" jsonschema:"description=New monetary value"`
	Currency       string         `json:"currency,omitempty" jsonschema:"description=New currency code"`
	OwnerID        int64          `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	PersonID       int64          `json:"person_id,omitempty" jsonschema:"description=New person ID"`
	OrganizationID int64          `json:"organization_id,omitempty" jsonschema:"description=New organization ID"`
	PipelineID     int64          `json:"pipeline_id,omitempty" jsonschema:"description=New pipeline ID"`
	StageID        int64          `json:"stage_id,omitempty" jsonschema:"description=New stage ID"`
	Status         string         `json:"status,omitempty" jsonschema:"description=New status: open|won|lost"`
	LabelIDs       *[]int64       `json:"label_ids,omitempty" jsonschema:"description=Replace the deal's labels with these label option IDs (empty array clears all labels). Resolve option IDs from the deal label field options in pipedrive.context.get deal_fields"`
	CustomFields   map[string]any `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs to merge"`
}

type DealsDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Deal ID to delete"`
}

type DealsArchiveParams struct {
	ID int64 `json:"id" jsonschema:"description=Deal ID to archive"`
}

type DealsUnarchiveParams struct {
	ID int64 `json:"id" jsonschema:"description=Deal ID to unarchive"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func dealsList(ctx context.Context, args DealsListParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.list", guardRead); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	mode, err := pipedrive.ValidCacheMode(args.CacheMode)
	if err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	q, err := internal.AddV2Pagination(url.Values{}, args.Limit, args.Cursor)
	if err != nil {
		return nil, err
	}
	if args.Status != "" {
		q.Set("status", args.Status)
	}
	if args.OwnerID != 0 {
		q.Set("owner_id", strconv.FormatInt(args.OwnerID, 10))
	}
	if args.PipelineID != 0 {
		q.Set("pipeline_id", strconv.FormatInt(args.PipelineID, 10))
	}
	if args.StageID != 0 {
		q.Set("stage_id", strconv.FormatInt(args.StageID, 10))
	}
	if args.FilterID != 0 {
		q.Set("filter_id", strconv.FormatInt(args.FilterID, 10))
	}
	if args.UpdatedSince != "" {
		q.Set("updated_since", args.UpdatedSince)
	}
	if args.UpdatedUntil != "" {
		q.Set("updated_until", args.UpdatedUntil)
	}

	path := "/deals"
	if args.IsArchived != nil && *args.IsArchived {
		path = "/deals/archived"
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.List, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}

	raw, dealsArr := extractListData(payload)
	normalized := pipedrive.NormalizeDealList(dealsArr)

	meta := map[string]any{
		"limit": effectiveLimit(args.Limit),
		"cache": cacheMeta,
	}
	if nextCursor := extractNextCursor(raw); nextCursor != "" {
		meta["next_cursor"] = nextCursor
	}
	data := map[string]any{"deals": normalized}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, meta), nil
}

func dealsGet(ctx context.Context, args DealsGetParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.get", guardRead); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	mode, err := pipedrive.ValidCacheMode(args.CacheMode)
	if err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	path := "/deals/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Deal, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}

	raw, dealRaw := extractItemData(payload)
	deal := pipedrive.NormalizeDeal(dealRaw)
	data := map[string]any{"deal": deal}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func dealsSearch(ctx context.Context, args DealsSearchParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.search", guardRead); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if len([]rune(args.Term)) < 2 {
		return nil, fmt.Errorf("term must be at least 2 characters")
	}
	mode, err := pipedrive.ValidCacheMode(args.CacheMode)
	if err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("term", args.Term)
	if args.Fields != "" {
		q.Set("fields", args.Fields)
	}
	if args.Status != "" {
		q.Set("status", args.Status)
	}
	if args.Exact {
		q.Set("exact_match", "true")
	}
	if _, err := internal.AddV2Pagination(q, args.Limit, ""); err != nil {
		return nil, err
	}

	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/deals/search", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/deals/search", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Search, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}

	return internal.Wrap(
		map[string]any{"results": internal.MaskSensitive(payload)},
		map[string]any{"cache": cacheMeta, "limit": effectiveLimit(args.Limit)},
	), nil
}

func dealsCreate(ctx context.Context, args DealsCreateParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.create", guardWrite); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if err := internal.RequireID(args.Title, "title"); err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{"title": args.Title}
	setIfNonZeroFloat(body, "value", args.Value)
	setIfNonZero(body, "currency", args.Currency)
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZeroInt(body, "pipeline_id", args.PipelineID)
	setIfNonZeroInt(body, "stage_id", args.StageID)
	setIfNonZero(body, "status", args.Status)
	if len(args.CustomFields) > 0 {
		body["custom_fields"] = args.CustomFields
	}

	req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/deals", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateDealsCache(client, 0)
	raw, dealRaw := extractItemData(payload)
	deal := pipedrive.NormalizeDeal(dealRaw)
	return internal.Wrap(map[string]any{"deal": deal, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func dealsUpdate(ctx context.Context, args DealsUpdateParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.update", guardWrite); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{}
	setIfNonZero(body, "title", args.Title)
	if args.Value != nil {
		body["value"] = *args.Value
	}
	setIfNonZero(body, "currency", args.Currency)
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZeroInt(body, "pipeline_id", args.PipelineID)
	setIfNonZeroInt(body, "stage_id", args.StageID)
	setIfNonZero(body, "status", args.Status)
	if len(args.CustomFields) > 0 {
		body["custom_fields"] = args.CustomFields
	}
	if args.LabelIDs != nil {
		body["label_ids"] = *args.LabelIDs
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	path := "/deals/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateDealsCache(client, args.ID)
	raw, dealRaw := extractItemData(payload)
	deal := pipedrive.NormalizeDeal(dealRaw)
	return internal.Wrap(map[string]any{"deal": deal, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func dealsDelete(ctx context.Context, args DealsDeleteParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.deals.delete", guardDelete); err != nil {
		return nil, err
	} else if disabled != nil {
		return disabled, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	path := "/deals/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateDealsCache(client, args.ID)
	return internal.Wrap(map[string]any{
		"deleted": true,
		"id":      args.ID,
		"raw":     internal.MaskSensitive(payload),
	}, nil), nil
}

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractNextCursor(payload any) string {
	m, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	ai, ok := m["additional_data"].(map[string]any)
	if !ok {
		return ""
	}
	if nc, ok := ai["next_cursor"].(string); ok {
		return nc
	}
	return ""
}

func effectiveLimit(v int) int {
	if v <= 0 {
		return internal.DefaultLimit
	}
	if v > internal.MaxLimit {
		return internal.MaxLimit
	}
	return v
}

func setIfNonZero(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func setIfNonZeroInt(m map[string]any, key string, val int64) {
	if val != 0 {
		m[key] = val
	}
}

func setIfNonZeroFloat(m map[string]any, key string, val float64) {
	if val != 0 {
		m[key] = val
	}
}

// invalidateDealsCache removes cached deal entries after a write.
// Conservatively invalidates:
//   - the single-deal GET (when id is known)
//   - every GET under /deals
//   - search endpoints under /deals/search
//
// We don't touch other resources' caches here.
func invalidateDealsCache(client *pipedrive.Client, id int64) {
	if client == nil {
		return
	}
	if id > 0 {
		path := "/deals/" + strconv.FormatInt(id, 10)
		client.InvalidatePath(pipedrive.V2, http.MethodGet, path)
		client.InvalidatePath(pipedrive.V1, http.MethodGet, path)
	}
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/deals")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/deals")
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/deals/search")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/deals/search")
}

// wrapAPIError surfaces a readable upstream error while preserving the HTTP
// status in the message. Returning a *HardError would hide the details from
// the LLM, so we prefer a soft error.
func wrapAPIError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *pipedrive.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	// Keep a short body snippet so the LLM can react to validation errors.
	snippet := apiErr.Body
	if len(snippet) > 512 {
		snippet = snippet[:512]
	}
	var pretty any
	if json.Unmarshal(snippet, &pretty) == nil {
		return fmt.Errorf("pipedrive api error %d: %v", apiErr.StatusCode, pretty)
	}
	return fmt.Errorf("pipedrive api error %d: %s", apiErr.StatusCode, string(snippet))
}

// ---------------------------------------------------------------------------
// Tool declarations
// ---------------------------------------------------------------------------

var DealsList = mcppipedrive.MustTool(
	"pipedrive.deals.list",
	"List deals (Pipedrive API v2) with optional filter_id, status, owner/pipeline/stage, updated_since/updated_until, sort_by/sort_direction, is_archived, and cursor pagination. When paginating a filtered call, re-pass filter_id with every cursor.",
	dealsList,
	mcp.WithTitleAnnotation("List deals"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

var DealsGet = mcppipedrive.MustTool(
	"pipedrive.deals.get",
	"Retrieve a single deal by ID (Pipedrive API v2).",
	dealsGet,
	mcp.WithTitleAnnotation("Get deal"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

var DealsSearch = mcppipedrive.MustTool(
	"pipedrive.deals.search",
	"Search deals by term (Pipedrive API v2).",
	dealsSearch,
	mcp.WithTitleAnnotation("Search deals"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

var DealsCreate = mcppipedrive.MustTool(
	"pipedrive.deals.create",
	"Create a new deal (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealsCreate,
	mcp.WithTitleAnnotation("Create deal"),
)

var DealsUpdate = mcppipedrive.MustTool(
	"pipedrive.deals.update",
	"Update an existing deal (write), including label_ids for deal labels. Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealsUpdate,
	mcp.WithTitleAnnotation("Update deal"),
)

var DealsDelete = mcppipedrive.MustTool(
	"pipedrive.deals.delete",
	"Delete a deal (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	dealsDelete,
	mcp.WithTitleAnnotation("Delete deal"),
	mcp.WithDestructiveHintAnnotation(true),
)

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

