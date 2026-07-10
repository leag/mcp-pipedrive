package tools

import (
	"context"
	"fmt"

	mcppipedrive "mcp-pipedrive"
	"mcp-pipedrive/pipedrive"
)

// ensureToolAllowed combines three guardrails:
//  1. The tool is in the allowlist (if any).
//  2. Write tools require PIPEDRIVE_ALLOW_WRITE=true.
//  3. Delete tools require BOTH write and delete flags.
//
// Returns structured errors so the LLM sees a readable tool result, not a
// protocol-level failure. The calling handler must `return result, nil` for
// the disabled cases (structured error as tool payload, as specified in the
// implementation brief).
type guardKind int

const (
	guardRead guardKind = iota
	guardWrite
	guardDelete
	guardAdmin
)

// disabledResult is the structured error payload returned when a gated tool is
// called but the corresponding flag is disabled.
type disabledResult struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func ensureToolAllowed(ctx context.Context, toolName string, kind guardKind) (any, error) {
	cfg := pipedrive.ConfigFromContext(ctx)

	if !pipedrive.IsToolAllowed(cfg, toolName) {
		return nil, fmt.Errorf("tool not allowed: %s", toolName)
	}

	switch kind {
	case guardWrite:
		if !cfg.AllowWrite {
			return disabledResult{
				Error:   "write_disabled",
				Message: "Set PIPEDRIVE_ALLOW_WRITE=true to enable write operations",
			}, nil
		}
	case guardDelete:
		if !cfg.AllowWrite {
			return disabledResult{
				Error:   "write_disabled",
				Message: "Delete tools require BOTH PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true; write is currently disabled",
			}, nil
		}
		if !cfg.AllowDelete {
			return disabledResult{
				Error:   "delete_disabled",
				Message: "Delete tools require BOTH PIPEDRIVE_ALLOW_WRITE=true AND PIPEDRIVE_ALLOW_DELETE=true; delete is currently disabled",
			}, nil
		}
	case guardAdmin:
		if !cfg.AdminToolsEnabled {
			return disabledResult{
				Error:   "admin_tools_disabled",
				Message: "Set PIPEDRIVE_ENABLE_ADMIN_TOOLS=true to enable admin tools",
			}, nil
		}
	}
	return nil, nil
}

// clientOrError fetches the Pipedrive client from context or returns a hard
// error. Handlers should call this after ensureToolAllowed.
func clientOrError(ctx context.Context) (*pipedrive.Client, error) {
	client := pipedrive.ClientFromContext(ctx)
	if client == nil {
		return nil, &mcppipedrive.HardError{Err: pipedrive.ErrMissingClient}
	}
	if client.AuthMode == pipedrive.AuthModeNone {
		return nil, pipedrive.ErrMissingAuth
	}
	return client, nil
}

// extractListData returns (rawPayload, dataArray) for a Pipedrive list
// response shaped as {"success":true,"data":[...]}.
func extractListData(payload any) (any, []any) {
	m, ok := payload.(map[string]any)
	if !ok {
		return payload, nil
	}
	if arr, ok := m["data"].([]any); ok {
		return m, arr
	}
	return m, nil
}

// extractItemData returns (rawPayload, dataObject) for a single-item response.
func extractItemData(payload any) (any, map[string]any) {
	m, ok := payload.(map[string]any)
	if !ok {
		return payload, nil
	}
	if d, ok := m["data"].(map[string]any); ok {
		return m, d
	}
	return m, m
}
