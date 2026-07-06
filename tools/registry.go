package tools

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/server"

	mcppipedrive "mcp-pipedrive"
)

// RegistrationConfig controls which tools get registered on the MCP server at
// startup. This complements the handler-level guards (ensureToolAllowed) and
// provides real token savings by keeping disabled tools out of the
// tools/list response entirely.
type RegistrationConfig struct {
	// AllowedTools, when non-empty, limits registration to the listed names.
	// An empty or nil map registers every non-admin tool.
	AllowedTools map[string]struct{}

	// AdminToolsEnabled controls registration of the destructive cache
	// admin tools (cache.clear, cache.invalidate). cache.stats is always
	// exposed because it is read-only and useful for diagnostics.
	AdminToolsEnabled bool
}

// adminToolNames are the destructive cache tools gated by
// PIPEDRIVE_ENABLE_ADMIN_TOOLS. cache.stats is deliberately excluded — it is
// read-only and a core diagnostic surface.
var adminToolNames = map[string]struct{}{
	"pipedrive.cache.clear":      {},
	"pipedrive.cache.invalidate": {},
}

// allTools is the authoritative list of every tool the server can expose.
// Order here determines the order in tools/list when no allowlist is set.
func allTools() []mcppipedrive.Tool {
	return []mcppipedrive.Tool{
		// context
		ContextGet,
		// deals core
		DealsList, DealsGet, DealsSearch, DealsCreate, DealsUpdate, DealsDelete, DealsArchive, DealsUnarchive,
		// deals.products
		DealProductsList, DealProductsAttach, DealProductsUpdate, DealProductsDetach,
		// deals.followers
		DealFollowersList, DealFollowersAdd, DealFollowersRemove,
		// persons core
		PersonsList, PersonsGet, PersonsSearch, PersonsCreate, PersonsUpdate, PersonsDelete,
		// persons.followers
		PersonFollowersList, PersonFollowersAdd, PersonFollowersRemove,
		// organizations core
		OrganizationsList, OrganizationsGet, OrganizationsSearch, OrganizationsCreate, OrganizationsUpdate, OrganizationsDelete,
		// organizations.followers
		OrgFollowersList, OrgFollowersAdd, OrgFollowersRemove,
		// products
		ProductsList, ProductsGet, ProductsSearch, ProductsCreate, ProductsUpdate, ProductsDelete,
		// leads
		LeadsList, LeadsGet, LeadsSearch, LeadsCreate, LeadsUpdate, LeadsDelete,
		// activities
		ActivitiesList, ActivitiesGet, ActivitiesCreate, ActivitiesUpdate, ActivitiesDelete, ActivitiesBatchCreate,
		// deal fields (option management)
		DealFieldsAddOption,
		// notes
		NotesList, NotesCreate,
		// filters (saved-filter discovery + management)
		FiltersList, FiltersCreate, FiltersUpdate,
		// mailbox
		MailThreadsList, MailThreadsGet, MailMessagesGet, DealsMailList,
		// webhooks
		WebhooksList, WebhooksCreate, WebhooksDelete,
		// cache (stats is always on; clear/invalidate behind AdminToolsEnabled)
		CacheStats, CacheClear, CacheInvalidate,
	}
}

// AddAllTools registers the configured subset of tools on the MCP server.
// It logs a warning for each entry in AllowedTools that doesn't match any
// known tool (almost always a typo).
func AddAllTools(m *server.MCPServer, cfg RegistrationConfig) {
	all := allTools()
	known := make(map[string]struct{}, len(all))
	for _, t := range all {
		known[t.Tool.Name] = struct{}{}
	}

	var registered, skippedAdmin, skippedAllowlist int
	for _, t := range all {
		name := t.Tool.Name
		if _, isAdmin := adminToolNames[name]; isAdmin && !cfg.AdminToolsEnabled {
			skippedAdmin++
			continue
		}
		if len(cfg.AllowedTools) > 0 {
			if _, ok := cfg.AllowedTools[name]; !ok {
				skippedAllowlist++
				continue
			}
		}
		t.Register(m)
		registered++
	}

	for name := range cfg.AllowedTools {
		if _, ok := known[name]; !ok {
			slog.Warn("PIPEDRIVE_ALLOWED_TOOLS entry does not match any known tool", "tool", name)
		}
	}

	slog.Info("tools registered",
		"count", registered,
		"skipped_admin", skippedAdmin,
		"skipped_allowlist", skippedAllowlist,
	)
}
