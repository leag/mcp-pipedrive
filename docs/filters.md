# Saved filters

Pipedrive's UI is built around **saved filters** — named, multi-criteria views (labels, custom fields, date ranges, status, owner). The MCP exposes them with one discovery tool plus a `filter_id` query parameter on every applicable list tool, so an LLM (or a human via tool args) can reproduce a UI view exactly.

| Tool | Underlying endpoint | Returns | Default TTL |
| --- | --- | --- | --- |
| `pipedrive.filters.list` | `GET /v1/filters` | `{id, name, type, active, visible_to, add_time, update_time}` per filter | `METADATA_TTL` 12h |

The full `conditions` tree is intentionally dropped from the normalized response — the LLM only needs `id`+`name`+`type` to apply a filter. Pass `include_raw: true` to get the raw v1 payload (conditions and all).

## Discovery → application

```json
{ "name": "pipedrive.filters.list" }
```

Then take an `id` and pass it as `filter_id`:

```json
{
  "name": "pipedrive.deals.list",
  "arguments": { "filter_id": 1 }
}
```

Filters are typed. The `type` field tells you which list tool the filter applies to:

| Filter `type` | Use with |
| --- | --- |
| `deals` | `pipedrive.deals.list` |
| `people` | `pipedrive.persons.list` |
| `org` | `pipedrive.organizations.list` |
| `activity` | `pipedrive.activities.list` |
| `product` | `pipedrive.products.list` |
| `lead` | `pipedrive.leads.list` |

Mismatching the type (e.g. applying a `people` filter to `pipedrive.deals.list`) surfaces a clean upstream `400 filter_id: Filter not found` via `wrapAPIError` — it does **not** silently return unfiltered data.

Narrow the discovery call by type when you already know what you want:

```json
{ "name": "pipedrive.filters.list", "arguments": { "type": "deals" } }
```

## Native date filters (no saved filter needed)

For the common "what changed recently / what's due today" pattern, prefer native date params over a saved filter — they're cheaper and don't require discovery:

| Tool | Date params |
| --- | --- |
| `pipedrive.deals.list` | `updated_since`, `updated_until` (RFC3339) |
| `pipedrive.persons.list` | `updated_since`, `updated_until` |
| `pipedrive.organizations.list` | `updated_since`, `updated_until` |
| `pipedrive.activities.list` | `updated_since`, `updated_until`, `due_date` (YYYY-MM-DD) |
| `pipedrive.products.list` | `updated_since`, `updated_until` |
| `pipedrive.notes.list` | `start_date`, `end_date` (YYYY-MM-DD, filters by `add_time`) |
| `pipedrive.leads.list` | none — v1 `/leads` has no native date filtering. Use a `filter_id` for date-bounded lead views. |

For non-`update_time` dimensions (`won_time`, `expected_close_date`, `add_time`, etc.), there is no v2 list parameter — those must go through a saved filter.

## Pagination caveat

The v2 cursor encodes a position in the **filtered** result stream. When you page through a filtered call, re-pass `filter_id` with every cursor:

```json
// page 1
{ "name": "pipedrive.deals.list", "arguments": { "filter_id": 1 } }
// → response includes meta.next_cursor

// page 2 — RE-PASS filter_id
{ "name": "pipedrive.deals.list", "arguments": { "filter_id": 1, "cursor": "…" } }
```

Dropping `filter_id` on the second call silently mixes filtered + unfiltered pages.

## Cache TTL

`pipedrive.filters.list` is cached at `PIPEDRIVE_CACHE_METADATA_TTL` (default 12h). Filters change rarely (humans edit them in the UI), so a long TTL keeps the discovery tool nearly free. The trade-off: if a teammate edits or deletes a filter in the Pipedrive UI, your MCP discovery may lag by up to 12h.

Bust it explicitly when needed:

```json
{
  "name": "pipedrive.cache.invalidate",
  "arguments": {
    "path": "/api/v1/filters",
    "method": "GET"
  }
}
```

Or force-refresh on a single call:

```json
{
  "name": "pipedrive.filters.list",
  "arguments": { "cache_mode": "refresh" }
}
```

## What is *not* exposed

There is no `pipedrive.filters.{get,delete}`. Filter deletion stays in the Pipedrive UI. `pipedrive.filters.create` and `pipedrive.filters.update` exist but are write-gated (require `PIPEDRIVE_ALLOW_WRITE=true`). See docs/filters.md for details. For filter discovery and inspection, call `pipedrive.filters.list` with `include_raw: true` to read raw conditions.

There is also no native label / custom-field filter on the list tools themselves. Both of those are exactly what saved filters are for — build the view once in the UI, then call by `filter_id`.
