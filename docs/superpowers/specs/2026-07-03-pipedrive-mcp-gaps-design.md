# Design: Pipedrive MCP gaps (mary TODO.md)

**Date:** 2026-07-03
**Source:** `TODO.md` on the `pipedrive-mcp-gaps` branch of the mary repo — gaps found while writing the `sales-lead-prequalification` skill.
**Scope decisions (confirmed with Luis):**

- All 13 MCP-implementable items are in scope (blocking + nice-to-have + schema re-review).
- Delete gating: **document only** — keep the compound `ALLOW_WRITE` + `ALLOW_DELETE` requirement, fix the error message and README.
- Field management: **enum-option add only** — no general dealFields create/update.
- Custom-field filtering: **saved-filter tools** (`filters.create` / `filters.update`), not in-MCP post-filtering. This also absorbs the `deals.search` structured-filters item.
- Structure: **one branch, six grouped commits**, one PR.
- Out of scope (Pipedrive API limitation, documented in the skill): sending email via API.

## Section 1 — Tool surface

### New tools (9)

All follow the repo's three-part pattern: params struct with `jsonschema` tags → handler opening with `ensureToolAllowed` + `clientOrError` → `mcppipedrive.MustTool` var appended to `allTools()` in `tools/registry.go`.

| Tool | Endpoint | Gate |
|---|---|---|
| `pipedrive.deals.archive` | `POST /api/v2/deals/{id}/archive` | write |
| `pipedrive.deals.unarchive` | `POST /api/v2/deals/{id}/unarchive` | write |
| `pipedrive.deal_fields.add_option` | v1 `GET /dealFields/{id}` then `PUT /dealFields/{id}` | write |
| `pipedrive.filters.create` | v1 `POST /filters` | write |
| `pipedrive.filters.update` | v1 `PUT /filters/{id}` | write |
| `pipedrive.activities.batch_create` | v2 `POST /activities`, looped server-side | write |
| `pipedrive.webhooks.list` | v1 `GET /webhooks` | read |
| `pipedrive.webhooks.create` | v1 `POST /webhooks` | write |
| `pipedrive.webhooks.delete` | v1 `DELETE /webhooks/{id}` | delete (write + delete flags, existing convention) |

### Changed tools (5)

1. **`deals.update`** — new `label_ids` param (`*[]int64`; pointer distinguishes "clear all labels" (`[]`) from "not provided" (nil)).
2. **`deals.list`** — new params: `is_archived` (`*bool`), `sort_by` (`id | add_time | update_time`), `sort_direction` (`asc | desc`). With `sort_by=add_time` a caller can scan by creation date and stop paging at the cutoff — this is the TODO's accepted alternative to native `add_time` range params, which v2 `GET /deals` does not offer.
3. **`deals.get` / `deals.list`** — normalized deal objects surface `label_ids` and `is_archived`.
4. **`context.get`** — fifth parallel metadata fetch: v1 `GET /activityTypes` → `activity_types` key (cached with `TTLs.Metadata`, same per-key error isolation as the other four). Fixes the `activities.create` blind `email`→`task` fallback: the skill can validate the type key upfront. `activities.create`'s tool description gains a note to check `context.get` for valid `type` keys.
5. **Delete gating message** — `guardDelete` denial message states that BOTH `PIPEDRIVE_ALLOW_WRITE=true` and `PIPEDRIVE_ALLOW_DELETE=true` are required; README's gating section spells it out.

Registry: 59 → 68 tools. README catalog, tool-count references, and `docs/roles/*.md` recipes updated.

## Section 2 — Behavior details

### Deal archive / unarchive

- Params: `{id int64}` (required).
- On success return `internal.Wrap({"deal": normalized}, nil)` from the response body; invalidate via the existing `invalidateDealsCache(client, id)`.
- Tool annotations: `WithIdempotentHintAnnotation(true)` (archiving an archived deal is a no-op/4xx passed through as soft error).

### `deal_fields.add_option`

- Params: `{field_id int64, options []string}` (both required, options non-empty).
- Flow: `GET /dealFields/{id}` (bypass cache) → verify field type is `enum` or `set` (soft error otherwise) → append new options (skip labels that already exist, case-sensitive match; report them as `skipped` in the result) → `PUT /dealFields/{id}` with the merged options array.
- Result: `{field, added: [...], skipped: [...]}`. Invalidate v1 `/dealFields` cache (context.get reads it).
- Known Pipedrive quirk: the PUT must send the full options array (existing options carry their `id`, new ones are `{label}` only).

### `filters.create` / `filters.update`

- Create params: `{name string, type string (deals|leads|persons|org|products|activities), conditions object}`.
- Update params: `{id int64, name?, conditions?}`.
- `conditions` is passed through as raw JSON to the API. The tool description links to Pipedrive's filter-conditions format (two-level glue tree: `{"glue":"and","conditions":[{"glue":"or","conditions":[...]}, ...]}`) and gives one worked example using a custom-field key from `context.get`. No client-side validation beyond "is a JSON object" — Pipedrive's 400 body is surfaced via the existing `wrapAPIError` path so the LLM sees what was wrong.
- Invalidate v1 `/filters` cache on both.
- This is the designed answer to both "no server-side custom-field filtering" and "deals.search is term-based only": create a saved filter once, then `deals.list?filter_id=X` is true server-side filtering.

### `activities.batch_create`

- Params: `{activities: [ActivityInput], stop_on_error?: bool (default false)}` where `ActivityInput` mirrors `ActivitiesCreateParams` minus cache fields. Max 100 items (soft error above).
- Sequential execution through the existing client (rate limiter applies per request). Continue on per-item failure unless `stop_on_error`.
- Result: `{results: [{index, ok, activity?, error?}], succeeded: n, failed: m}`. Single cache invalidation for `/activities` at the end.
- Rationale: Pipedrive has no batch endpoint; the win is collapsing ~500 MCP round-trips per daily run into ~10 while keeping per-item error visibility.

### Webhooks

- `webhooks.list`: no params beyond `cache_mode`; cached with `TTLs.Metadata`.
- `webhooks.create` params: `{subscription_url (required), event_action (create|change|delete|*), event_object (deal|activity|person|organization|note|lead|product|*), user_id?, http_auth_user?, http_auth_password?, version? ("1.0"|"2.0", default "2.0")}`. Auth password masked in the echoed result via `internal.MaskSensitive`.
- `webhooks.delete` params: `{id int64}`; delete-gated.
- Create/delete invalidate the `/webhooks` cache.

### Mail direction (`deals.mail.list`)

- Investigation task: check `pipedrive/normalize_mail.go` for what party/direction data the normalizer keeps.
- Pipedrive mail messages expose `from`/`to`/`cc` party arrays and a `mail_thread` `first_message_direction`-style hint; the normalized message will carry an explicit `direction` field derived from party data when the API provides it (values: `incoming | outgoing | unknown`), plus the from/to parties, so reply detection stops depending on string-matching the lead's address.
- If the raw API turns out to already provide a reliable direction flag, we surface it verbatim instead of deriving.

### API-fact verification (before implementation)

During planning, verify against the Pipedrive API reference (docs pages, not memory): v2 archive endpoints' response shape, `is_archived` query param on v2 `GET /deals`, `label_ids` accepted by v2 `PATCH /deals/{id}`, v2 `GET /deals` `sort_by` allowed values, v1 `PUT /dealFields/{id}` options semantics, webhooks v2 `version` field. Any mismatch updates this spec before code is written.

## Section 3 — Testing, docs, delivery

### Testing (per repo conventions, stdlib `testing` only)

- Query/body propagation tests with the `captureTransport` pattern in `tools/list_params_query_test.go` (or a sibling file) for every new tool and every new param: assert method, path, query, and JSON body.
- `batch_create` test uses a scripted transport returning success/failure alternately; asserts per-item results, ordering, and `stop_on_error` behavior.
- Guard tests in `tools/guardrails_test.go` for each new gated tool (write off → `write_disabled`; delete tool with only one flag → improved compound message).
- Registry test (`tools/registry_test.go`): assert the 9 new names appear in `tools/list`, count updated to 68.
- Normalizer tests for `label_ids`/`is_archived` on deals and `direction` on mail messages.
- `go build ./... && go test ./...` green; dry-run suite untouched (new read tools may gain dryrun cases but that's optional).

### Docs

- README: catalog +9 tools with gate tiers, tool-count updates, delete-gating section ("delete tools require BOTH flags"), short webhooks + batch sections.
- `docs/filters.md`: add create/update usage and the conditions-format example.
- `docs/roles/*.md`: add the new tools to the relevant role recipes (e.g. the mary/prequalification role gets archive/unarchive, batch_create, add_option, filters.create).
- No change to mary's TODO.md in this repo (it lives in the mary repo; ticking it off happens there after merge).

### Delivery

- Branch: `feat/mary-todo-gaps` off current `main` of mcp-pipedrive.
- Six commits, TDD each: (1) deal lifecycle (archive/unarchive + is_archived), (2) labels, (3) metadata (activity types in context.get + add_option), (4) listing/filtering (sort params + filters.create/update), (5) throughput/eventing (batch_create + webhooks), (6) docs/polish (mail direction, gating message, README/roles).
- One PR to `main`; code-review pass before requesting merge.
