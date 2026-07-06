package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registeredNames extracts the tool names registered on a fresh MCPServer by
// replaying the tools/list request through the mcp-go list handler. It's the
// same surface the client sees, so it tests the real outcome of AddAllTools.
func registeredNames(t *testing.T, cfg RegistrationConfig) []string {
	t.Helper()
	s := server.NewMCPServer("test", "0")
	AddAllTools(s, cfg)

	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcp.NewRequestId(1),
		Request: mcp.Request{Method: "tools/list"},
	}
	raw, _ := json.Marshal(req)
	resp := s.HandleMessage(t.Context(), raw)
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	names := make([]string, 0, len(parsed.Result.Tools))
	for _, tool := range parsed.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestRegistry_DefaultRegistersEverythingExceptDestructiveAdmin(t *testing.T) {
	names := registeredNames(t, RegistrationConfig{})
	if len(names) == 0 {
		t.Fatalf("expected tools to be registered")
	}
	// A sampling of read/write tools must be present by default.
	for _, must := range []string{
		"pipedrive.context.get",
		"pipedrive.deals.list",
		"pipedrive.deals.create",
		"pipedrive.deals.archive",
		"pipedrive.deals.unarchive",
		"pipedrive.persons.search",
		"pipedrive.filters.list",
		"pipedrive.filters.create",
		"pipedrive.filters.update",
		"pipedrive.deal_fields.add_option",
		"pipedrive.activities.batch_create",
		"pipedrive.webhooks.list",
		"pipedrive.webhooks.create",
		"pipedrive.webhooks.delete",
		"pipedrive.cache.stats",
	} {
		if !contains(names, must) {
			t.Errorf("missing expected default tool: %s", must)
		}
	}
	// cache.clear / cache.invalidate must be hidden until admin flag is on.
	for _, hidden := range []string{"pipedrive.cache.clear", "pipedrive.cache.invalidate"} {
		if contains(names, hidden) {
			t.Errorf("%s must not be registered unless AdminToolsEnabled is true", hidden)
		}
	}
}

func TestRegistry_AdminFlagExposesCacheWriteTools(t *testing.T) {
	names := registeredNames(t, RegistrationConfig{AdminToolsEnabled: true})
	for _, must := range []string{"pipedrive.cache.clear", "pipedrive.cache.invalidate"} {
		if !contains(names, must) {
			t.Errorf("admin tool %s should be registered when flag is on", must)
		}
	}
}

func TestRegistry_AllowlistRestrictsRegisteredSet(t *testing.T) {
	allowed := map[string]struct{}{
		"pipedrive.deals.list": {},
		"pipedrive.deals.get":  {},
	}
	names := registeredNames(t, RegistrationConfig{AllowedTools: allowed})

	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(names), names)
	}
	if !contains(names, "pipedrive.deals.list") || !contains(names, "pipedrive.deals.get") {
		t.Errorf("allowlist entries not registered: %v", names)
	}
	if contains(names, "pipedrive.deals.create") {
		t.Errorf("unlisted tool slipped through allowlist: %v", names)
	}
}

func TestRegistry_AllowlistCannotSurfaceAdminWithoutFlag(t *testing.T) {
	// Allowlist contains cache.clear but admin flag is off → still hidden.
	allowed := map[string]struct{}{
		"pipedrive.cache.clear": {},
	}
	names := registeredNames(t, RegistrationConfig{AllowedTools: allowed, AdminToolsEnabled: false})
	if contains(names, "pipedrive.cache.clear") {
		t.Errorf("admin tool must stay hidden even when present in allowlist if AdminToolsEnabled=false")
	}
	if len(names) != 0 {
		t.Errorf("expected 0 tools, got %d: %v", len(names), names)
	}
}
