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

type PersonsListParams struct {
	Limit          int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"description=Cursor from a previous page"`
	OwnerID        int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by owner user ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Filter by organization ID"`
	FilterID       int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	UpdatedSince   string `json:"updated_since,omitempty" jsonschema:"description=RFC3339 lower bound on update_time"`
	UpdatedUntil   string `json:"updated_until,omitempty" jsonschema:"description=RFC3339 upper bound on update_time"`
	IncludeRaw     bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode      string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type PersonsGetParams struct {
	ID         int64  `json:"id" jsonschema:"description=Person ID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type PersonsSearchParams struct {
	Term           string `json:"term" jsonschema:"description=Search term (minimum 2 characters)"`
	Fields         string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields: name|email|phone|custom_fields"`
	Exact          bool   `json:"exact_match,omitempty" jsonschema:"description=Require exact match"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Restrict to a specific organization"`
	Limit          int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	CacheMode      string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type PersonsCreateParams struct {
	Name           string              `json:"name" jsonschema:"description=Full name"`
	Emails         []pipedrive.Contact `json:"emails,omitempty" jsonschema:"description=List of {value,label,primary} email entries"`
	Phones         []pipedrive.Contact `json:"phones,omitempty" jsonschema:"description=List of {value,label,primary} phone entries"`
	OwnerID        int64               `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	OrganizationID int64               `json:"organization_id,omitempty" jsonschema:"description=Linked organization ID"`
	CustomFields   map[string]any      `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type PersonsUpdateParams struct {
	ID             int64               `json:"id" jsonschema:"description=Person ID to update"`
	Name           string              `json:"name,omitempty" jsonschema:"description=New full name"`
	Emails         []pipedrive.Contact `json:"emails,omitempty" jsonschema:"description=Replace email list"`
	Phones         []pipedrive.Contact `json:"phones,omitempty" jsonschema:"description=Replace phone list"`
	OwnerID        int64               `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	OrganizationID int64               `json:"organization_id,omitempty" jsonschema:"description=New organization ID"`
	CustomFields   map[string]any      `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs to merge. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type PersonsDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Person ID to delete"`
}

func personsList(ctx context.Context, args PersonsListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.list", guardRead); err != nil {
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
	if args.OrganizationID != 0 {
		q.Set("org_id", strconv.FormatInt(args.OrganizationID, 10))
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
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/persons", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/persons", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.List, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	items := pipedrive.NormalizePersonList(arr)
	decodeCustomFields(client, pipedrive.FieldEntityPerson, mode, items)
	data := map[string]any{"persons": items}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "cache": cacheMeta}
	if nc := extractNextCursor(raw); nc != "" {
		meta["next_cursor"] = nc
	}
	return internal.Wrap(data, meta), nil
}

func personsGet(ctx context.Context, args PersonsGetParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.get", guardRead); err != nil {
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
	path := "/persons/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Person, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizePerson(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityPerson, mode, &item)
	data := map[string]any{"person": item}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func personsSearch(ctx context.Context, args PersonsSearchParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.search", guardRead); err != nil {
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
	if args.OrganizationID != 0 {
		q.Set("organization_id", strconv.FormatInt(args.OrganizationID, 10))
	}
	if _, err := internal.AddV2Pagination(q, args.Limit, ""); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/persons/search", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/persons/search", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Search, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return internal.Wrap(
		map[string]any{"results": internal.MaskSensitive(payload)},
		map[string]any{"cache": cacheMeta, "limit": effectiveLimit(args.Limit)},
	), nil
}

func personsCreate(ctx context.Context, args PersonsCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.create", guardWrite); err != nil {
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
	if len(args.Emails) > 0 {
		body["emails"] = args.Emails
	}
	if len(args.Phones) > 0 {
		body["phones"] = args.Phones
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	if err := setCustomFields(client, pipedrive.FieldEntityPerson, body, args.CustomFields); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/persons", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidatePersonsCache(client, 0)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizePerson(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityPerson, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"person": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func personsUpdate(ctx context.Context, args PersonsUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.update", guardWrite); err != nil {
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
	if len(args.Emails) > 0 {
		body["emails"] = args.Emails
	}
	if len(args.Phones) > 0 {
		body["phones"] = args.Phones
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	if err := setCustomFields(client, pipedrive.FieldEntityPerson, body, args.CustomFields); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/persons/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidatePersonsCache(client, args.ID)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizePerson(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityPerson, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"person": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func personsDelete(ctx context.Context, args PersonsDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.persons.delete", guardDelete); err != nil {
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
	path := "/persons/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidatePersonsCache(client, args.ID)
	return internal.Wrap(map[string]any{"deleted": true, "id": args.ID, "raw": internal.MaskSensitive(payload)}, nil), nil
}

func invalidatePersonsCache(client *pipedrive.Client, id int64) {
	if client == nil {
		return
	}
	if id > 0 {
		path := "/persons/" + strconv.FormatInt(id, 10)
		client.InvalidatePath(pipedrive.V2, http.MethodGet, path)
		client.InvalidatePath(pipedrive.V1, http.MethodGet, path)
	}
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/persons")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/persons")
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/persons/search")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/persons/search")
}

var PersonsList = mcppipedrive.MustTool("pipedrive.persons.list",
	"List persons (Pipedrive API v2) with cursor pagination and filter_id / owner / org / updated_since-until filters. When paginating a filtered call, re-pass filter_id with every cursor.",
	personsList,
	mcp.WithTitleAnnotation("List persons"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var PersonsGet = mcppipedrive.MustTool("pipedrive.persons.get",
	"Retrieve a single person by ID (Pipedrive API v2).",
	personsGet,
	mcp.WithTitleAnnotation("Get person"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var PersonsSearch = mcppipedrive.MustTool("pipedrive.persons.search",
	"Search persons by term (Pipedrive API v2).",
	personsSearch,
	mcp.WithTitleAnnotation("Search persons"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var PersonsCreate = mcppipedrive.MustTool("pipedrive.persons.create",
	"Create a new person (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	personsCreate,
	mcp.WithTitleAnnotation("Create person"))

var PersonsUpdate = mcppipedrive.MustTool("pipedrive.persons.update",
	"Update an existing person (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	personsUpdate,
	mcp.WithTitleAnnotation("Update person"))

var PersonsDelete = mcppipedrive.MustTool("pipedrive.persons.delete",
	"Delete a person (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	personsDelete,
	mcp.WithTitleAnnotation("Delete person"), mcp.WithDestructiveHintAnnotation(true))
