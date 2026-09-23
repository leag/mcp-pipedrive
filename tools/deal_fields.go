package tools

// Deal-field option management. Pipedrive's v1 PUT /dealFields/{id} replaces
// the FULL options array: existing options must be resent with their id, new
// options carry only a label. This tool wraps the read-merge-write cycle so
// callers can't accidentally drop existing options.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/internal"
	"mcp-pipedrive/pipedrive"
)

type DealFieldsAddOptionParams struct {
	FieldID int64    `json:"field_id" jsonschema:"description=Numeric deal field ID (from pipedrive.context.get deal_fields)"`
	Options []string `json:"options" jsonschema:"description=Option labels to append to the enum/set field (existing labels are skipped)"`
}

func dealFieldsAddOption(ctx context.Context, args DealFieldsAddOptionParams) (any, error) {
	if d, err := ensureToolAllowed(ctx, "pipedrive.deal_fields.add_option", guardWrite); err != nil {
		return nil, err
	} else if d != nil {
		return d, nil
	}
	if args.FieldID <= 0 {
		return nil, fmt.Errorf("field_id is required and must be > 0")
	}
	if len(args.Options) == 0 {
		return nil, fmt.Errorf("options must contain at least one label")
	}
	for _, label := range args.Options {
		if strings.TrimSpace(label) == "" {
			return nil, fmt.Errorf("options must not contain empty or whitespace-only labels")
		}
	}
	client, err := clientOrError(ctx)
	if err != nil {
		return nil, err
	}

	path := "/dealFields/" + strconv.FormatInt(args.FieldID, 10)
	getReq, err := client.NewRequest(pipedrive.V1, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var getPayload any
	if err := client.DoJSON(getReq.WithContext(ctx), &getPayload); err != nil {
		return nil, wrapAPIError(err)
	}
	_, field := extractItemData(getPayload)
	fieldType := toStringRaw(field["field_type"])
	if fieldType != "enum" && fieldType != "set" {
		return nil, fmt.Errorf("field %d has field_type %q; options can only be added to enum or set fields", args.FieldID, fieldType)
	}

	existing, _ := field["options"].([]any)
	merged := make([]any, 0, len(existing)+len(args.Options))
	have := map[string]bool{}
	for _, o := range existing {
		om, ok := o.(map[string]any)
		if !ok {
			continue
		}
		label := toStringRaw(om["label"])
		have[label] = true
		merged = append(merged, map[string]any{"id": om["id"], "label": label})
	}
	added := []string{}
	skipped := []string{}
	for _, label := range args.Options {
		if have[label] {
			skipped = append(skipped, label)
			continue
		}
		have[label] = true
		added = append(added, label)
		merged = append(merged, map[string]any{"label": label})
	}
	if len(added) == 0 {
		return internal.Wrap(map[string]any{
			"field":   internal.MaskSensitive(field),
			"added":   added,
			"skipped": skipped,
		}, nil), nil
	}

	putReq, err := client.NewRequest(pipedrive.V1, http.MethodPut, path, nil, map[string]any{"options": merged})
	if err != nil {
		return nil, err
	}
	var putPayload any
	if err := client.DoJSON(putReq.WithContext(ctx), &putPayload); err != nil {
		return nil, wrapAPIError(err)
	}
	client.InvalidatePath(pipedrive.V1, http.MethodGet, "/dealFields")
	_, updated := extractItemData(putPayload)
	return internal.Wrap(map[string]any{
		"field":   internal.MaskSensitive(updated),
		"added":   added,
		"skipped": skipped,
	}, nil), nil
}

var DealFieldsAddOption = mcppipedrive.MustTool(
	"pipedrive.deal_fields.add_option",
	"Append option labels to an enum/set deal field (write; read-merge-write on v1 /dealFields). Requires PIPEDRIVE_ALLOW_WRITE=true.",
	dealFieldsAddOption,
	mcp.WithTitleAnnotation("Add deal-field option"),
)
