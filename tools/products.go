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

type ProductsListParams struct {
	Limit        int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"description=Cursor from a previous page"`
	OwnerID      int64  `json:"owner_id,omitempty" jsonschema:"description=Filter by owner user ID"`
	Active       *bool  `json:"active,omitempty" jsonschema:"description=Filter by active flag"`
	FilterID     int64  `json:"filter_id,omitempty" jsonschema:"description=Apply a saved Pipedrive filter (discover via pipedrive.filters.list)"`
	UpdatedSince string `json:"updated_since,omitempty" jsonschema:"description=RFC3339 lower bound on update_time"`
	UpdatedUntil string `json:"updated_until,omitempty" jsonschema:"description=RFC3339 upper bound on update_time"`
	IncludeRaw   bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode    string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type ProductsGetParams struct {
	ID         int64  `json:"id" jsonschema:"description=Product ID"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v2 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type ProductsSearchParams struct {
	Term      string `json:"term" jsonschema:"description=Search term (minimum 2 characters)"`
	Fields    string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields: name|code|custom_fields"`
	Exact     bool   `json:"exact_match,omitempty" jsonschema:"description=Require exact match"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Max results (1-500 default 50)"`
	CacheMode string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type ProductsCreateParams struct {
	Name         string                   `json:"name" jsonschema:"description=Product name"`
	Code         string                   `json:"code,omitempty" jsonschema:"description=Product code/SKU"`
	Unit         string                   `json:"unit,omitempty" jsonschema:"description=Unit of measure (e.g. hour piece)"`
	Active       *bool                    `json:"active,omitempty" jsonschema:"description=Active flag (default true)"`
	OwnerID      int64                    `json:"owner_id,omitempty" jsonschema:"description=Owner user ID"`
	Prices       []pipedrive.ProductPrice `json:"prices,omitempty" jsonschema:"description=Per-currency prices"`
	CustomFields map[string]any           `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type ProductsUpdateParams struct {
	ID           int64                    `json:"id" jsonschema:"description=Product ID to update"`
	Name         string                   `json:"name,omitempty" jsonschema:"description=New name"`
	Code         string                   `json:"code,omitempty" jsonschema:"description=New code/SKU"`
	Unit         string                   `json:"unit,omitempty" jsonschema:"description=New unit"`
	Active       *bool                    `json:"active,omitempty" jsonschema:"description=New active flag"`
	OwnerID      int64                    `json:"owner_id,omitempty" jsonschema:"description=New owner user ID"`
	Prices       []pipedrive.ProductPrice `json:"prices,omitempty" jsonschema:"description=Replace per-currency prices"`
	CustomFields map[string]any           `json:"custom_fields,omitempty" jsonschema:"description=Custom field key-value pairs to merge. Keys may be the field name as returned by reads (e.g. '# of devices') or the 40-char field key; enum/set values may be the option label (e.g. 'full_suite') or its numeric ID"`
}

type ProductsDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Product ID to delete"`
}

func productsList(ctx context.Context, args ProductsListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.list", guardRead); err != nil {
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
	if args.Active != nil {
		q.Set("active", strconv.FormatBool(*args.Active))
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
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/products", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/products", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Product, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	items := pipedrive.NormalizeProductList(arr)
	decodeCustomFields(client, pipedrive.FieldEntityProduct, mode, items)
	data := map[string]any{"products": items}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	meta := map[string]any{"limit": effectiveLimit(args.Limit), "cache": cacheMeta}
	if nc := extractNextCursor(raw); nc != "" {
		meta["next_cursor"] = nc
	}
	return internal.Wrap(data, meta), nil
}

func productsGet(ctx context.Context, args ProductsGetParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.get", guardRead); err != nil {
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
	path := "/products/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, path, nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Product, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeProduct(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityProduct, mode, &item)
	data := map[string]any{"product": item}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func productsSearch(ctx context.Context, args ProductsSearchParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.search", guardRead); err != nil {
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
	req, err := client.NewRequest(pipedrive.V2, http.MethodGet, "/products/search", q, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V2, http.MethodGet, "/products/search", q, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Search, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return internal.Wrap(
		map[string]any{"results": internal.MaskSensitive(payload)},
		map[string]any{"cache": cacheMeta, "limit": effectiveLimit(args.Limit)},
	), nil
}

func productsCreate(ctx context.Context, args ProductsCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.create", guardWrite); err != nil {
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
	setIfNonZero(body, "code", args.Code)
	setIfNonZero(body, "unit", args.Unit)
	if args.Active != nil {
		body["active"] = *args.Active
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	if len(args.Prices) > 0 {
		body["prices"] = args.Prices
	}
	if err := setCustomFields(client, pipedrive.FieldEntityProduct, body, args.CustomFields); err != nil {
		return nil, err
	}
	req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/products", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateProductsCache(client, 0)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeProduct(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityProduct, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"product": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func productsUpdate(ctx context.Context, args ProductsUpdateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.update", guardWrite); err != nil {
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
	setIfNonZero(body, "code", args.Code)
	setIfNonZero(body, "unit", args.Unit)
	if args.Active != nil {
		body["active"] = *args.Active
	}
	setIfNonZeroInt(body, "owner_id", args.OwnerID)
	if len(args.Prices) > 0 {
		body["prices"] = args.Prices
	}
	if err := setCustomFields(client, pipedrive.FieldEntityProduct, body, args.CustomFields); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	path := "/products/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateProductsCache(client, args.ID)
	raw, m := extractItemData(payload)
	item := pipedrive.NormalizeProduct(m)
	decodeOneCustomFields(client, pipedrive.FieldEntityProduct, pipedrive.CacheModeDefault, &item)
	return internal.Wrap(map[string]any{"product": item, "raw": internal.MaskSensitive(raw)}, nil), nil
}

func productsDelete(ctx context.Context, args ProductsDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.products.delete", guardDelete); err != nil {
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
	path := "/products/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V2, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	invalidateProductsCache(client, args.ID)
	return internal.Wrap(map[string]any{"deleted": true, "id": args.ID, "raw": internal.MaskSensitive(payload)}, nil), nil
}

func invalidateProductsCache(client *pipedrive.Client, id int64) {
	if client == nil {
		return
	}
	if id > 0 {
		path := "/products/" + strconv.FormatInt(id, 10)
		client.InvalidatePath(pipedrive.V2, http.MethodGet, path)
		client.InvalidatePath(pipedrive.V1, http.MethodGet, path)
	}
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/products")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/products")
	client.InvalidatePath(pipedrive.V2, http.MethodGet, "/products/search")
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/products/search")
}

var ProductsList = mcppipedrive.MustTool("pipedrive.products.list",
	"List products (Pipedrive API v2) with cursor pagination and filter_id / owner / active / updated_since-until filters. When paginating a filtered call, re-pass filter_id with every cursor.",
	productsList,
	mcp.WithTitleAnnotation("List products"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var ProductsGet = mcppipedrive.MustTool("pipedrive.products.get",
	"Retrieve a single product by ID (Pipedrive API v2).",
	productsGet,
	mcp.WithTitleAnnotation("Get product"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var ProductsSearch = mcppipedrive.MustTool("pipedrive.products.search",
	"Search products by term (Pipedrive API v2).",
	productsSearch,
	mcp.WithTitleAnnotation("Search products"), mcp.WithIdempotentHintAnnotation(true), mcp.WithReadOnlyHintAnnotation(true))

var ProductsCreate = mcppipedrive.MustTool("pipedrive.products.create",
	"Create a new product (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	productsCreate,
	mcp.WithTitleAnnotation("Create product"))

var ProductsUpdate = mcppipedrive.MustTool("pipedrive.products.update",
	"Update an existing product (write). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	productsUpdate,
	mcp.WithTitleAnnotation("Update product"))

var ProductsDelete = mcppipedrive.MustTool("pipedrive.products.delete",
	"Delete a product (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	productsDelete,
	mcp.WithTitleAnnotation("Delete product"), mcp.WithDestructiveHintAnnotation(true))
