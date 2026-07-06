package tools

// Webhook subscription management (v1 — v2 has no webhooks surface).
// The MCP only manages subscriptions; receiving the callbacks requires a
// publicly reachable HTTPS endpoint (e.g. an n8n Webhook trigger URL).

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type WebhooksListParams struct {
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"description=If true also include raw v1 payload"`
	CacheMode  string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

type WebhooksCreateParams struct {
	Name             string `json:"name" jsonschema:"description=Webhook name (max 255 chars)"`
	SubscriptionURL  string `json:"subscription_url" jsonschema:"description=Public HTTPS endpoint Pipedrive will POST events to"`
	EventAction      string `json:"event_action" jsonschema:"description=Event action: create|change|delete|*"`
	EventObject      string `json:"event_object" jsonschema:"description=Event object: activity|deal|lead|note|organization|person|pipeline|product|stage|user|*"`
	UserID           int64  `json:"user_id,omitempty" jsonschema:"description=Only send events visible to this user ID (default: creator)"`
	HTTPAuthUser     string `json:"http_auth_user,omitempty" jsonschema:"description=HTTP basic auth username for the receiver"`
	HTTPAuthPassword string `json:"http_auth_password,omitempty" jsonschema:"description=HTTP basic auth password for the receiver"`
	Version          string `json:"version,omitempty" jsonschema:"description=Webhook payload version: 1.0|2.0 (default 2.0)"`
}

type WebhooksDeleteParams struct {
	ID int64 `json:"id" jsonschema:"description=Webhook ID to delete (discover via pipedrive.webhooks.list)"`
}

func webhooksList(ctx context.Context, args WebhooksListParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.list", guardRead); err != nil {
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
	req, err := client.NewRequest(pipedrive.V1, http.MethodGet, "/webhooks", nil, nil)
	if err != nil {
		return nil, err
	}
	key := pipedrive.Key(client, pipedrive.V1, http.MethodGet, "/webhooks", nil, nil)
	payload, cacheMeta, err := client.CachedGet(req.WithContext(ctx), key, client.TTLs.Metadata, mode)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	raw, arr := extractListData(payload)
	data := map[string]any{"webhooks": pipedrive.NormalizeWebhookList(arr)}
	if args.IncludeRaw {
		data["raw"] = internal.MaskSensitive(raw)
	}
	return internal.Wrap(data, map[string]any{"cache": cacheMeta}), nil
}

func webhooksCreate(ctx context.Context, args WebhooksCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if err := internal.RequireID(args.Name, "name"); err != nil {
		return nil, err
	}
	if err := internal.RequireID(args.SubscriptionURL, "subscription_url"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.EventAction, "event_action", "create", "change", "delete", "*"); err != nil {
		return nil, err
	}
	if err := internal.RequireOneOf(args.EventObject, "event_object",
		"activity", "deal", "lead", "note", "organization", "person", "pipeline", "product", "stage", "user", "*"); err != nil {
		return nil, err
	}
	if args.Version != "" && args.Version != "1.0" && args.Version != "2.0" {
		return nil, fmt.Errorf("version must be 1.0 or 2.0")
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":             args.Name,
		"subscription_url": args.SubscriptionURL,
		"event_action":     args.EventAction,
		"event_object":     args.EventObject,
	}
	setIfNonZeroInt(body, "user_id", args.UserID)
	setIfNonZero(body, "http_auth_user", args.HTTPAuthUser)
	setIfNonZero(body, "http_auth_password", args.HTTPAuthPassword)
	setIfNonZero(body, "version", args.Version)

	req, err := client.NewRequest(pipedrive.V1, http.MethodPost, "/webhooks", nil, body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/webhooks")
	raw, m := extractItemData(payload)
	return internal.Wrap(map[string]any{
		"webhook": pipedrive.NormalizeWebhook(m),
		"raw":     internal.MaskSensitive(raw),
	}, nil), nil
}

func webhooksDelete(ctx context.Context, args WebhooksDeleteParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.webhooks.delete", guardDelete); err != nil {
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
	path := "/webhooks/" + strconv.FormatInt(args.ID, 10)
	req, err := client.NewRequest(pipedrive.V1, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/webhooks")
	return internal.Wrap(map[string]any{
		"deleted": true,
		"id":      args.ID,
		"raw":     internal.MaskSensitive(payload),
	}, nil), nil
}

var WebhooksList = mcppipedrive.MustTool("pipedrive.webhooks.list",
	"List webhook subscriptions (v1).",
	webhooksList,
	mcp.WithTitleAnnotation("List webhooks"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true))

var WebhooksCreate = mcppipedrive.MustTool("pipedrive.webhooks.create",
	"Create a webhook subscription (write; Pipedrive POSTs matching events to subscription_url). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	webhooksCreate,
	mcp.WithTitleAnnotation("Create webhook"))

var WebhooksDelete = mcppipedrive.MustTool("pipedrive.webhooks.delete",
	"Delete a webhook subscription (write+delete). Requires PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true.",
	webhooksDelete,
	mcp.WithTitleAnnotation("Delete webhook"),
	mcp.WithDestructiveHintAnnotation(true))
