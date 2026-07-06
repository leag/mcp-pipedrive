package tools

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type ContextGetParams struct {
	CacheMode string `json:"cache_mode,omitempty" jsonschema:"description=Cache mode: default|bypass|refresh|only"`
}

// contextGet returns users + pipelines + stages + deal-fields + activity-types metadata in one
// call. All five sub-calls share the metadata TTL.
func contextGet(ctx context.Context, args ContextGetParams) (any, error) {
	if disabled, err := ensureToolAllowed(ctx, "pipedrive.context.get", guardRead); err != nil {
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

	type result struct {
		key     string
		payload any
		meta    *pipedrive.CacheMeta
		err     error
	}

	calls := []struct {
		version pipedrive.APIVersion
		path    string
		extra   url.Values
		key     string
	}{
		{pipedrive.V1, "/users", nil, "users"},
		{pipedrive.V2, "/pipelines", nil, "pipelines"},
		{pipedrive.V2, "/stages", nil, "stages"},
		{pipedrive.V1, "/dealFields", nil, "deal_fields"},
		{pipedrive.V1, "/activityTypes", nil, "activity_types"},
	}

	out := make([]result, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(i int, v pipedrive.APIVersion, path, key string, q url.Values) {
			defer wg.Done()
			req, err := client.NewRequest(v, http.MethodGet, path, q, nil)
			if err != nil {
				out[i] = result{key: key, err: err}
				return
			}
			ck := pipedrive.Key(client, v, http.MethodGet, path, q, nil)
			payload, meta, err := client.CachedGet(req.WithContext(ctx), ck, client.TTLs.Metadata, mode)
			out[i] = result{key: key, payload: payload, meta: meta, err: err}
		}(i, c.version, c.path, c.key, c.extra)
	}
	wg.Wait()

	data := map[string]any{}
	cacheBlock := map[string]any{}
	for _, r := range out {
		if r.err != nil {
			data[r.key] = map[string]any{"error": wrapAPIError(r.err).Error()}
			continue
		}
		data[r.key] = internal.MaskSensitive(extractDataField(r.payload))
		cacheBlock[r.key] = r.meta
	}
	return internal.Wrap(data, map[string]any{"cache": cacheBlock}), nil
}

// extractDataField unwraps the `data` field when present; otherwise returns payload as-is.
func extractDataField(payload any) any {
	if m, ok := payload.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			return d
		}
	}
	return payload
}

var ContextGet = mcppipedrive.MustTool(
	"pipedrive.context.get",
	"Fetch users, pipelines, stages, deal-field and activity-type metadata in one cached call. Useful to prime an LLM before deal operations.",
	contextGet,
	mcp.WithTitleAnnotation("Get Pipedrive context"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)
