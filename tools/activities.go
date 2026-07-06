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

type ActivitiesListParams struct {
	Limit          int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"description=Cursor from a previous page"`
	OwnerID        int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by owner user ID"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=Filter by deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=Filter by person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Filter by organization ID"`
	Done           *bool  `json:"done,omitempty" jsonschema:"description=Filter by done=true|false"`
	FilterID       int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	DueDate        string `json:"due_date,omitempty" jsonschema:"description=Filter by due date YYYY-MM-DD"`
	UpdatedSince   string `json:"updated_since,omitempty" jsonschema:"description=RFC3339 lower bound on update_time"`
	UpdatedUntil   string `json:"updated_until,omitempty" jsonschema:"description=RFC3339 upper bound on update_time"`
	IncludeRaw     bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode      string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type ActivitiesGetParams struct {
	ID         int64  `json:"id" jsonschema:"description=Activity ID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type ActivitiesCreateParams struct {
	Subject        string `json:"subject" jsonschema:"description=Activity subject/title"`
	Type           string `json:"type,omitempty" jsonschema:"description=Activity type key (e.g. call|meeting|email)"`
	DueDate        string `json:"due_date,omitempty" jsonschema:"description=Due date YYYY-MM-DD"`
	DueTime        string `json:"due_time,omitempty" jsonschema:"description=Due time HH:MM"`
	Duration       string `json:"duration,omitempty" jsonschema:"description=Duration HH:MM"`
	Done           bool   `json:"done,omitempty" jsonschema:"description=Mark the activity as done"`
	OwnerID        int64  `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=Linked deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=Linked person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=Linked organization ID"`
	Note           string `json:"note,omitempty" jsonschema:"description=Free-form note attached to the activity"`
}

type ActivitiesUpdateParams struct {
	ID             int64  `json:"id" jsonschema:"description=Activity ID to update"`
	Subject        string `json:"subject,omitempty" jsonschema:"description=New subject"`
	Type           string `json:"type,omitempty" jsonschema:"description=New type key"`
	DueDate        string `json:"due_date,omitempty" jsonschema:"description=New due date YYYY-MM-DD"`
	DueTime        string `json:"due_time,omitempty" jsonschema:"description=New due time HH:MM"`
	Duration       string `json:"duration,omitempty" jsonschema:"description=New duration HH:MM"`
	Done           *bool  `json:"done,omitempty" jsonschema:"description=Set done=true|false"`
	OwnerID        int64  `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	DealID         int64  `json:"deal_id,omitempty" jsonschema:"description=New linked deal ID"`
	PersonID       int64  `json:"person_id,omitempty" jsonschema:"description=New linked person ID"`
	OrganizationID int64  `json:"organization_id,omitempty" jsonschema:"description=New linked organization ID"`
	Note           string `json:"note,omitempty" jsonschema:"description=New note"`
}

type ActivitiesDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Activity ID to delete"`
}

func activitiesList(ctx context.Context, args ActivitiesListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.list", guardRead); err != nil {
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
	if args.DealID != 0 {
		q.Set("deal_id", strconv.FormatInt(args.DealID, 10))
	}
	if args.PersonID != 0 {
		q.Set("person_id", strconv.FormatInt(args.PersonID, 10))
	}
	if args.OrganizationID != 0 {
		q.Set("org_id", strconv.FormatInt(args.OrganizationID, 10))
	}
	if args.Done != nil {
		q.Set("done", strconv.FormatBool(*args.Done))
	}
	if args.FilterID != 0 {
		q.Set("filter_id", strconv.FormatInt(args.FilterID, 10))
	}
	if args.DueDate != "" {
		q.Set("due_date", args.DueDate)
	}
	if args.UpdatedSince != "" {
		q.Set("updated_since", args.UpdatedSince)
	}
	if args.UpdatedUntil != "" {
		q.Set("updated_until", args.UpdatedUntil)
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/activities", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/activities", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Activity, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	data := map[string]any{"activities": pipedrive.NormalizeActivityList(arr)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "cache": cacheMeta}
	if nc := extractNextCursor(raw); nc != "" {
		meta["next_cursor"] = nc
	}
	return internal.Wrap(data, meta), nil
}

func activitiesGet(ctx context.Context, args ActivitiesGetParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.get", guardRead); err != nil {
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
	path := "/activities/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Activity, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, m := extractItemData(payload)
	data := map[string]any{"activity": pipedrive.NormalizeActivity(m)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

// activityCreateBody maps create params to the v2 POST /activities body.
// Shared by activities.create and activities.batch_create.
func activityCreateBody(args ActivitiesCreateParams) map[string]any {
	body := map[string]any{"subject": args.Subject}
	setIfNonZero(body, "type", args.Type)
	setIfNonZero(body, "due_date", args.DueDate)
	setIfNonZero(body, "due_time", args.DueTime)
	setIfNonZero(body, "duration", args.Duration)
	if args.Done {
		body["done"] = true
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "deal_id", args.DealID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZero(body, "note", args.Note)
	return body
}

func activitiesCreate(ctx context.Context, args ActivitiesCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Subject, "subject"); err != nil {
		return nil, err
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := activityCreateBody(args)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/activities", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateActivitiesCache(client, 0)
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{"activity": pipedrive.NormalizeActivity(m), "raw": internal.MaskSensitive(raw)}, nil), nil
}

func activitiesUpdate(ctx context.Context, args ActivitiesUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.update", guardWrite); err != nil {
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
	setIfNonZero(body, "subject", args.Subject)
	setIfNonZero(body, "type", args.Type)
	setIfNonZero(body, "due_date", args.DueDate)
	setIfNonZero(body, "due_time", args.DueTime)
	setIfNonZero(body, "duration", args.Duration)
	if args.Done != nil {
		body["done"] = *args.Done
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	setIfNonZeroInt(body, "deal_id", args.DealID)
	setIfNonZeroInt(body, "person_id", args.PersonID)
	setIfNonZeroInt(body, "org_id", args.OrganizationID)
	setIfNonZero(body, "note", args.Note)
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/activities/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateActivitiesCache(client, args.ID)
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{"activity": pipedrive.NormalizeActivity(m), "raw": internal.MaskSensitive(raw)}, nil), nil
}

func activitiesDelete(ctx context.Context, args ActivitiesDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.delete", guardDelete); err != nil {
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
	path := "/activities/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateActivitiesCache(client, args.ID)
	return internal.Wrap(map[string]any{"deleted": true, "id": args.ID, "raw": internal.MaskSensitive(payload)}, nil), nil
}

func invalidateActivitiesCache(client *pipedrive.Client, id int64) {
	if client == nil {
		return
	}
	if id > 0 {
		path := "/activities/" + strconv.FormatInt(id, 10)
		client.InvalidatePath(pipedrive.V2, http.MethodGet, path)
		client.InvalidatePath(pipedrive.V1, http.MethodGet, path)
	}
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/activities")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/activities")
}

var ActivitiesList = mcppipedrive.MustTool("pipedrive.activities.list",
	"List activities (Pipedrive API v2) with cursor pagination and filter_id, link filters, due_date, and updated_since/updated_until. When paginating a filtered call, re-pass filter_id with every cursor.",
	activitiesList,
	mcp.WithTitleAnnotation("List activities"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var ActivitiesGet = mcppipedrive.MustTool("pipedrive.activities.get",
	"Retrieve a single activity by ID (Pipedrive API v2).",
	activitiesGet,
	mcp.WithTitleAnnotation("Get activity"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var ActivitiesCreate = mcppipedrive.MustTool("pipedrive.activities.create",
	"Create a new activity (write). Requires PIPEDRIVE_ALLOW_WRITE=true. Validate the type key against pipedrive.context.get activity_types[].key_string first — unknown types are rejected by Pipedrive.",
	activitiesCreate,
	mcp.WithTitleAnnotation("Create activity"))

var ActivitiesUpdate = mcppipedrive.MustTool("pipedrive.activities.update",
	"Update an existing activity (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	activitiesUpdate,
	mcp.WithTitleAnnotation("Update activity"))

var ActivitiesDelete = mcppipedrive.MustTool("pipedrive.activities.delete",
	"Delete an activity (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	activitiesDelete,
	mcp.WithTitleAnnotation("Delete activity"), mcp.WithDestructiveHintAnnotation(true))
