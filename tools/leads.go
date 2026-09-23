package tools

// Leads are v1-only at the time of writing. Lead IDs are UUID strings (not
// numeric like deals). Lead value is an object {amount, currency}.

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

type LeadsListParams struct {
	Limit      int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Start      int    `json:"start,omitempty" jsonschema:"description=Offset for pagination (v1 uses start)"`
	OwnerID    int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by owner user ID"`
	Archived   *bool  `json:"archived,omitempty" jsonschema:"description=Filter archived status"`
	FilterID   int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list). For date-bounded views use a filter."`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type LeadsGetParams struct {
	ID         string `json:"id" jsonschema:"description=Lead UUID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type LeadsSearchParams struct {
	Term      string `json:"term" jsonschema:"description=Search term (minimum 2 characters)"`
	Fields    string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields: title|notes|custom_fields"`
	Exact     bool   `json:"exact_match,omitempty" jsonschema:"description=Require exact match"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	CacheMode string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type LeadsCreateParams struct {
	Title             string   `json:"title" jsonschema:"description=Lead title"`
	OwnerID           int64    `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	PersonID          int64    `json:"person_id,omitempty" jsonschema:"description=Linked person ID"`
	OrganizationID    int64    `json:"organization_id,omitempty" jsonschema:"description=Linked organization ID"`
	ValueAmount       float64  `json:"value_amount,omitempty" jsonschema:"description=Lead value amount"`
	ValueCurrency     string   `json:"value_currency,omitempty" jsonschema:"description=Lead value currency code (e.g. USD EUR)"`
	ExpectedCloseDate string   `json:"expected_close_date,omitempty" jsonschema:"description=Expected close date YYYY-MM-DD"`
	LabelIDs          []string `json:"label_ids,omitempty" jsonschema:"description=Array of lead label UUIDs"`
}

type LeadsUpdateParams struct {
	ID                string   `json:"id" jsonschema:"description=Lead UUID to update"`
	Title             string   `json:"title,omitempty" jsonschema:"description=New title"`
	OwnerID           int64    `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	PersonID          int64    `json:"person_id,omitempty" jsonschema:"description=New linked person ID"`
	OrganizationID    int64    `json:"organization_id,omitempty" jsonschema:"description=New linked organization ID"`
	ValueAmount       *float64 `json:"value_amount,omitempty" jsonschema:"description=New value amount"`
	ValueCurrency     string   `json:"value_currency,omitempty" jsonschema:"description=New value currency code"`
	ExpectedCloseDate string   `json:"expected_close_date,omitempty" jsonschema:"description=New expected close date YYYY-MM-DD"`
	LabelIDs          []string `json:"label_ids,omitempty" jsonschema:"description=Replace label ID list"`
	IsArchived        *bool    `json:"is_archived,omitempty" jsonschema:"description=Archive/unarchive the lead"`
}

type LeadsDeleteParams struct {
	ID string `json:"id" jsonschema:"description=Lead UUID to delete"`
}

func leadsList(ctx context.Context, args LeadsListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.list", guardRead); err != nil {
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
	if args.OwnerID != 0 {
		q.Set("owner_id", strconv.FormatInt(args.OwnerID, 10))
	}
	if args.Archived != nil {
		q.Set("archived_status", archivedFilter(*args.Archived))
	}
	if args.FilterID != 0 {
		q.Set("filter_id", strconv.FormatInt(args.FilterID, 10))
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/leads", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/leads", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.List, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	items := pipedrive.NormalizeLeadList(arr)
	decodeCustomFields(client, pipedrive.FieldEntityDeal, mode, items)
	data := map[string]any{"leads": items}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "start": args.Start, "cache": cacheMeta}
	return internal.Wrap(data, meta), nil
}

func archivedFilter(archived bool) string {
	if archived {
		return "archived"
	}
	return "not_archived"
}

func leadsGet(ctx context.Context, args LeadsGetParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.get", guardRead); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.ID, "id"); err != nil {
		return nil, err
	}
	mode, err := pipedrive.ValidCacheMode(args.CacheMode)
	if err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	path := "/leads/" + args.ID
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Deal, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeLead(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityDeal, mode, &item)
	data := map[string]any{"lead": item}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func leadsSearch(ctx context.Context, args LeadsSearchParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.search", guardRead); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
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
	if args.Exact {
		q.Set("exact_match", "true")
	}
	// v1 search uses start + limit; treat limit as first-page size.
	if _, err := internal.AddV1Pagination(q, 0, args.Limit); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/leads/search", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/leads/search", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Search, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return internal.Wrap(
		map[string]any{"results": internal.MaskSensitive(payload)},
		map[string]any{"cache": cacheMeta, "limit": effectiveLimit(args.Limit)},
	), nil
}

func leadsCreate(ctx context.Context, args LeadsCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Title, "title"); err != nil {
		return nil, err
	}
	if args.PersonID == 0 && args.OrganizationID == 0 {
		return nil, fmt.Errorf("person_id or organization_id is required by Pipedrive to create a lead")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"title": args.Title}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "organization_id", args.OrganizationID)
	if args.ValueAmount != 0 || args.ValueCurrency != "" {
		body["value"] = map[string]any{"amount": args.ValueAmount, "currency": args.ValueCurrency}
	}
	setIfNonZero(body, "expected_close_date", args.ExpectedCloseDate)
	if len(args.LabelIDs) > 0 {
		body["label_ids"] = args.LabelIDs
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/leads", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateLeadsCache(client, "")
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeLead(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityDeal, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"lead": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func leadsUpdate(ctx context.Context, args LeadsUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.update", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.ID, "id"); err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	setIfNonZero(body, "title", args.Title)
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "organization_id", args.OrganizationID)
	if args.ValueAmount != nil || args.ValueCurrency != "" {
		val := map[string]any{}
		if args.ValueAmount != nil {
			val["amount"] = *args.ValueAmount
		}
		if args.ValueCurrency != "" {
			val["currency"] = args.ValueCurrency
		}
		body["value"] = val
	}
	setIfNonZero(body, "expected_close_date", args.ExpectedCloseDate)
	if len(args.LabelIDs) > 0 {
		body["label_ids"] = args.LabelIDs
	}
	if args.IsArchived != nil {
		body["is_archived"] = *args.IsArchived
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/leads/" + args.ID
	req, err := client.NewRequest(pipedrive.V1, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateLeadsCache(client, args.ID)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeLead(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityDeal, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"lead": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func leadsDelete(ctx context.Context, args LeadsDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.leads.delete", guardDelete); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.ID, "id"); err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	path := "/leads/" + args.ID
	req, err := client.NewRequest(pipedrive.V1, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateLeadsCache(client, args.ID)
	return internal.Wrap(map[string]any{"deleted": true, "id": args.ID, "raw": internal.MaskSensitive(payload)}, nil), nil
}

func invalidateLeadsCache(client *pipedrive.Client, id string) {
	if client == nil {
		return
	}
	if id != "" {
		client.InvalidatePath(pipedrive.V1, http.MethodGet, "/leads/"+id)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/leads")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/leads/search")
}

var LeadsList = mcppipedrive.MustTool("pipedrive.leads.list",
	"List leads (Pipedrive API v1, no v2 equivalent) with optional filter_id / owner / archived filters. For date-bounded views use a saved filter via filter_id.",
	leadsList,
	mcp.WithTitleAnnotation("List leads"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var LeadsGet = mcppipedrive.MustTool("pipedrive.leads.get",
	"Retrieve a single lead by UUID (Pipedrive API v1).",
	leadsGet,
	mcp.WithTitleAnnotation("Get lead"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var LeadsSearch = mcppipedrive.MustTool("pipedrive.leads.search",
	"Search leads by term (Pipedrive API v1).",
	leadsSearch,
	mcp.WithTitleAnnotation("Search leads"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var LeadsCreate = mcppipedrive.MustTool("pipedrive.leads.create",
	"Create a new lead (write). Requires at least one of person_id or organization_id. Requires PIPEDRIVE_ALLOW_WRITE=true.",
	leadsCreate,
	mcp.WithTitleAnnotation("Create lead"))

var LeadsUpdate = mcppipedrive.MustTool("pipedrive.leads.update",
	"Update an existing lead (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	leadsUpdate,
	mcp.WithTitleAnnotation("Update lead"))

var LeadsDelete = mcppipedrive.MustTool("pipedrive.leads.delete",
	"Delete a lead (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	leadsDelete,
	mcp.WithTitleAnnotation("Delete lead"), mcp.WithDestructiveHintAnnotation(true))
