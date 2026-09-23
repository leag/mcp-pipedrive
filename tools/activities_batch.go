package tools

// Batch activity creation. Pipedrive has no batch endpoint, so this loops
// POST /activities server-side: one MCP call instead of N round-trips
// through the LLM. The client's rate limiter still applies per request.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

const batchCreateMax = 100

type ActivitiesBatchCreateParams struct {
	Activities  []ActivitiesCreateParams `json:"activities" jsonschema:"description=Activities to create in order (1-100 items; same fields as pipedrive.activities.create)"`
	StopOnError bool                     `json:"stop_on_error,omitempty" jsonschema:"description=Abort at the first failed item (default false: continue and report per-item errors)"`
}

type batchItemResult struct {
	Index    int    `json:"index"`
	OK       bool   `json:"ok"`
	Activity any    `json:"activity,omitempty"`
	Error    string `json:"error,omitempty"`
}

func activitiesBatchCreate(ctx context.Context, args ActivitiesBatchCreateParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.activities.batch_create", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if len(args.Activities) == 0 {
		return nil, fmt.Errorf("activities must contain at least one item")
	}
	if len(args.Activities) > batchCreateMax {
		return nil, fmt.Errorf("activities is limited to %d items per call, got %d", batchCreateMax, len(args.Activities))
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]batchItemResult, 0, len(args.Activities))
	succeeded, failed := 0, 0
	for i, item := range args.Activities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item.Subject == "" {
			failed++
			results = append(results, batchItemResult{Index: i, Error: "subject is required"})
			if args.StopOnError {
				break
			}
			continue
		}
		req, err := client.NewRequest(pipedrive.V2, http.MethodPost, "/activities", nil, activityCreateBody(item))
		if err != nil {
			return nil, err
		}
		var payload any
		if err := client.DoJSON(req.WithContext(ctx), &payload); err != nil {
			failed++
			results = append(results, batchItemResult{Index: i, Error: wrapAPIError(err).Error()})
			if args.StopOnError {
				break
			}
			continue
		}
		_, m := extractItemData(payload)
		succeeded++
		results = append(results, batchItemResult{Index: i, OK: true, Activity: pipedrive.NormalizeActivity(m)})
	}
	if succeeded > 0 {
		invalidateActivitiesCache(client, 0)
	}
	return internal.Wrap(map[string]any{
		"results":   results,
		"succeeded": succeeded,
		"failed":    failed,
	}, nil), nil
}

var ActivitiesBatchCreate = mcppipedrive.MustTool(
	"pipedrive.activities.batch_create",
	"Create up to 100 activities in one call (write; sequential POSTs with per-item results). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	activitiesBatchCreate,
	mcp.WithTitleAnnotation("Batch-create activities"),
)
