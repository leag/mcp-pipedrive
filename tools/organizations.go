package tools

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

type OrganizationsListParams struct {
	Limit        int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"description=Cursor from a previous page"`
	OwnerID      int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by owner user ID"`
	FilterID     int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	UpdatedSince string `json:"updated_since,omitempty" jsonschema:"description=RFC3339 lower bound on update_time"`
	UpdatedUntil string `json:"updated_until,omitempty" jsonschema:"description=RFC3339 upper bound on update_time"`
	IncludeRaw   bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode    string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type OrganizationsGetParams struct {
	ID         int64  `json:"id" jsonschema:"description=Organization ID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type OrganizationsSearchParams struct {
	Term      string `json:"term" jsonschema:"description=Search term (minimum 2 characters)"`
	Fields    string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields: name|address|custom_fields"`
	Exact     bool   `json:"exact_match,omitempty" jsonschema:"description=Require exact match"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	CacheMode string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type OrganizationsCreateParams struct {
	Name         string         `json:"name" jsonschema:"description=Organization name"`
	OwnerID      int64          `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	Address      string         `json:"address,omitempty" jsonschema:"description=Free-form postal address"`
	CustomFields map[string]any `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type OrganizationsUpdateParams struct {
	ID           int64          `json:"id" jsonschema:"description=Organization ID to update"`
	Name         string         `json:"name,omitempty" jsonschema:"description=New name"`
	OwnerID      int64          `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	Address      string         `json:"address,omitempty" jsonschema:"description=New address"`
	CustomFields map[string]any `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs to merge. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type OrganizationsDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Organization ID to delete"`
}

func organizationsList(ctx context.Context, args OrganizationsListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.list", guardRead); err != nil {
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
	q, err := internal.AddV2Pagination(url.Values{}, args.Limit, args.Cursor)
	if err != nil {
		return nil, err
	}
	if args.OwnerID != 0 {
		q.Set("owner_id", strconv.FormatInt(args.OwnerID, 10))
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
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/organizations", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/organizations", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.List, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	items := pipedrive.NormalizeOrganizationList(arr)
	decodeCustomFields[pipedrive.NormalizedOrganization](ctx, client, pipedrive.FieldEntityOrganization, mode, items)
	decodeCustomFields[pipedrive.NormalizedOrganization](ctx, client, pipedrive.FieldEntityOrganization, mode, items)
	data := map[string]any{"organizations": items}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "cache": cacheMeta}
	if nc := extractNextCursor(raw); nc != "" {
		meta["next_cursor"] = nc
	}
	return internal.Wrap(data, meta), nil
}

func organizationsGet(ctx context.Context, args OrganizationsGetParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.get", guardRead); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
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
	path := "/organizations/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Organization, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeOrganization(m)
	decodeOneCustomFields[pipedrive.NormalizedOrganization](ctx, client, pipedrive.FieldEntityOrganization, mode, &item)
	data := map[string]any{"organization": item}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func organizationsSearch(ctx context.Context, args OrganizationsSearchParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.search", guardRead); err != nil {
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
	if _, err := internal.AddV2Pagination(q, args.Limit, ""); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/organizations/search", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/organizations/search", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Search, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return internal.Wrap(
		map[string]any{"results": internal.MaskSensitive(payload)},
		map[string]any{"cache": cacheMeta, "limit": effectiveLimit(args.Limit)},
	), nil
}

func organizationsCreate(ctx context.Context, args OrganizationsCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Name, "name"); err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"name": args.Name}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZero(body, "address", args.Address)
	if err := setCustomFields(ctx, client, pipedrive.FieldEntityOrganization, body, args.CustomFields); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/organizations", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateOrganizationsCache(client, 0)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeOrganization(m)
	decodeOneCustomFields[pipedrive.NormalizedOrganization](ctx, client, pipedrive.FieldEntityOrganization, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"organization": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func organizationsUpdate(ctx context.Context, args OrganizationsUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.update", guardWrite); err != nil {
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
	setIfNonZero(body, "name", args.Name)
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZero(body, "address", args.Address)
	if err := setCustomFields(ctx, client, pipedrive.FieldEntityOrganization, body, args.CustomFields); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/organizations/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateOrganizationsCache(client, args.ID)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeOrganization(m)
	decodeOneCustomFields[pipedrive.NormalizedOrganization](ctx, client, pipedrive.FieldEntityOrganization, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"organization": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func organizationsDelete(ctx context.Context, args OrganizationsDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.organizations.delete", guardDelete); err != nil {
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
	path := "/organizations/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateOrganizationsCache(client, args.ID)
	return internal.Wrap(map[string]any{"deleted": true, "id": args.ID, "raw": internal.MaskSensitive(payload)}, nil), nil
}

func invalidateOrganizationsCache(client *pipedrive.Client, id int64) {
	if client == nil {
		return
	}
	if id > 0 {
		path := "/organizations/" + strconv.FormatInt(id, 10)
		client.InvalidatePath(pipedrive.V2, http.MethodGet, path)
		client.InvalidatePath(pipedrive.V1, http.MethodGet, path)
	}
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/organizations")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/organizations")
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/organizations/search")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/organizations/search")
}

var OrganizationsList = mcppipedrive.MustTool("pipedrive.organizations.list",
	"List organizations (Pipedrive API v2) with cursor pagination and filter_id / owner / updated_since-until filter. When paginating a filtered call, re-pass filter_id with every cursor.",
	organizationsList,
	mcp.WithTitleAnnotation("List organizations"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var OrganizationsGet = mcppipedrive.MustTool("pipedrive.organizations.get",
	"Retrieve a single organization by ID (Pipedrive API v2).",
	organizationsGet,
	mcp.WithTitleAnnotation("Get organization"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var OrganizationsSearch = mcppipedrive.MustTool("pipedrive.organizations.search",
	"Search organizations by term (Pipedrive API v2).",
	organizationsSearch,
	mcp.WithTitleAnnotation("Search organizations"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var OrganizationsCreate = mcppipedrive.MustTool("pipedrive.organizations.create",
	"Create a new organization (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	organizationsCreate,
	mcp.WithTitleAnnotation("Create organization"))

var OrganizationsUpdate = mcppipedrive.MustTool("pipedrive.organizations.update",
	"Update an existing organization (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	organizationsUpdate,
	mcp.WithTitleAnnotation("Update organization"))

var OrganizationsDelete = mcppipedrive.MustTool("pipedrive.organizations.delete",
	"Delete an organization (write+delete). Requires both PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	organizationsDelete,
	mcp.WithTitleAnnotation("Delete organization"), mcp.WithDestructiveHintAnnotation(true))
