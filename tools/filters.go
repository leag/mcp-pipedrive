package tools

// Saved filters are Pipedrive API v1 only. This tool lets callers discover
// filter IDs so they can be passed as `filter_id` to any list tool and
// reproduce a UI view exactly.

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

type FiltersListParams struct {
	Type       string `json:"type,omitempty" jsonschema:"description=Filter type: deals|leads|org|people|products|activity|projects (omit for all)"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload (with conditions tree)"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

func filtersList(ctx context.Context, args FiltersListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.filters.list", guardRead); err != nil {
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
	q := url.Values{}
	if args.Type != "" {
		q.Set("type", args.Type)
	}
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/filters", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/filters", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Metadata, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	data := map[string]any{"filters": pipedrive.NormalizeFilterList(arr)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

type FiltersCreateParams struct {
	Name       string         `json:"name" jsonschema:"description=Filter name"`
	Type       string         `json:"type" jsonschema:"description=Filter type: deals|leads|org|people|products|activity|projects"`
	Conditions map[string]any `json:"conditions" jsonschema:"description=Pipedrive two-level glue conditions tree (max 16 leaf conditions). See the pipedrive.filters.create tool description for a worked example and docs/filters.md for details"`
}

type FiltersUpdateParams struct {
	ID         int64          `json:"id" jsonschema:"description=Filter ID to update (discover via pipedrive.filters.list)"`
	Name       string         `json:"name,omitempty" jsonschema:"description=New filter name"`
	Conditions map[string]any `json:"conditions" jsonschema:"description=Full replacement conditions tree (required by the v1 API on every update). The filter's type cannot be changed"`
}

var filterTypes = []string{"deals", "leads", "org", "people", "products", "activity", "projects"}

func filtersCreate(ctx context.Context, args FiltersCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.filters.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Name, "name"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.Type, "type", filterTypes...); err != nil {
		return nil, err
	}
	if len(args.Conditions) == 0 {
		return nil, fmt.Errorf("conditions is required (Pipedrive glue tree, see tool description)")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"name": args.Name, "type": args.Type, "conditions": args.Conditions}
	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/filters", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/filters")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"filter": pipedrive.NormalizeFilter(m),
		"raw":    internal.MaskSensitive(raw),
	}, nil), nil
}

func filtersUpdate(ctx context.Context, args FiltersUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.filters.update", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if args.ID <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	if len(args.Conditions) == 0 {
		return nil, fmt.Errorf("conditions is required — the v1 PUT replaces the whole tree on every update")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"conditions": args.Conditions}
	if args.Name != "" {
		body["name"] = args.Name
	}
	path := "/filters/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V1, http.MethodPut, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/filters")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"filter": pipedrive.NormalizeFilter(m),
		"raw":    internal.MaskSensitive(raw),
	}, nil), nil
}

var FiltersList = mcppipedrive.MustTool("pipedrive.filters.list",
	"List saved filters (Pipedrive API v1). Use the returned id with filter_id on any list tool to reproduce a UI view.",
	filtersList,
	mcp.WithTitleAnnotation("List saved filters"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true))

var FiltersCreate = mcppipedrive.MustTool("pipedrive.filters.create",
	`Create a saved filter (write; v1). Combine with filter_id on list tools for server-side filtering, including custom-field conditions. Requires PIPEDRIVE_ALLOW_WRITE=true. See docs/filters.md.

conditions is a two-level glue tree (max 16 leaf conditions). Example — deals where custom field 12345 equals "open":
{"glue":"and","conditions":[{"glue":"and","conditions":[{"object":"deal","field_id":"12345","operator":"=","value":"open"}]},{"glue":"or","conditions":[]}]}
Resolve field_id values (numeric field IDs, not hash keys) from pipedrive.context.get -> deal_fields.`,
	filtersCreate,
	mcp.WithTitleAnnotation("Create saved filter"))

var FiltersUpdate = mcppipedrive.MustTool("pipedrive.filters.update",
	"Update a saved filter's name/conditions (write; v1; type is immutable, conditions fully replaced). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	filtersUpdate,
	mcp.WithTitleAnnotation("Update saved filter"))
