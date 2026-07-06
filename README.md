# mcp-pipedrive

MCP server exposing the Pipedrive CRM API as Model Context Protocol tools. v2-first with v1 fallback, dual authentication (API token or OAuth), write/delete guardrails, optional bbolt-backed cache.

> **Save LLM context: configure by role.** Listing all 54 tools costs ~8,950 tokens every time the LLM reloads the catalog. Pick a role in [`docs/roles.md`](docs/roles.md) to drop that to as low as ~3,600 tokens while keeping full capability for that role. Configurations are built from Pipedrive's official [permission sets](https://support.pipedrive.com/en/article/permission-sets) and [OAuth scopes](https://pipedrive.readme.io/docs/marketplace-scopes-and-permissions-explanations).

## Transports

- `stdio` (default) — Claude Desktop, Codex CLI, local subprocesses
- `sse` — HTTP Server-Sent Events
- `streamable-http` — modern HTTP streaming (recommended for remote)

## Authentication

Set exactly one of:

| Env var | Use for |
|---|---|
| `PIPEDRIVE_API_TOKEN` | Company-wide API token (passed as `?api_token=` query param) |
| `PIPEDRIVE_OAUTH_ACCESS_TOKEN` | Bearer token from an OAuth flow (passed as `Authorization: Bearer`) |

When both are set, OAuth wins.

Set `PIPEDRIVE_DOMAIN` to your Pipedrive subdomain, e.g. `mycompany.pipedrive.com`. For OAuth-scoped access use `api.pipedrive.com` (default).

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `PIPEDRIVE_DOMAIN` | `api.pipedrive.com` | Pipedrive host (no scheme) |
| `PIPEDRIVE_API_TOKEN` | — | Company API token |
| `PIPEDRIVE_OAUTH_ACCESS_TOKEN` | — | OAuth bearer token |
| `PIPEDRIVE_ALLOW_WRITE` | `false` | Enable create/update tools |
| `PIPEDRIVE_ALLOW_DELETE` | `false` | Enable delete tools (also requires ALLOW_WRITE) |
| `PIPEDRIVE_ALLOWED_TOOLS` | (empty = all) | Comma-separated allowlist. **Tools not in this list are not registered**, so they never appear in `tools/list` — real token savings for LLM contexts |
| `PIPEDRIVE_TIMEOUT_MS` | `30000` | HTTP client timeout (ms) |
| `PIPEDRIVE_RATE_LIMIT_DISABLE` | `false` | Skip the rate limiter |
| `PIPEDRIVE_DEBUG` | `false` | Verbose logging + args in spans |
| `PIPEDRIVE_ENABLE_ADMIN_TOOLS` | `false` | Register `cache.clear` / `cache.invalidate`. When false they are not exposed at all (not just handler-gated). `cache.stats` is always on. |

### Cache

| Env var | Default | Purpose |
|---|---|---|
| `PIPEDRIVE_CACHE_ENABLED` | `true` | Disable to bypass the cache entirely |
| `PIPEDRIVE_CACHE_PATH` | `.cache/pipedrive-mcp.bbolt` | bbolt file path |
| `PIPEDRIVE_CACHE_METADATA_TTL` | `12h` | users/pipelines/stages/fields, saved filters |
| `PIPEDRIVE_CACHE_DEAL_TTL` | `180s` | single deal GET |
| `PIPEDRIVE_CACHE_PERSON_TTL` | `600s` | single person GET |
| `PIPEDRIVE_CACHE_ORGANIZATION_TTL` | `600s` | single organization GET |
| `PIPEDRIVE_CACHE_ACTIVITY_TTL` | `60s` | activity GET/list |
| `PIPEDRIVE_CACHE_PRODUCT_TTL` | `30m` | product GET/list |
| `PIPEDRIVE_CACHE_FOLLOWER_TTL` | `300s` | follower lists |
| `PIPEDRIVE_CACHE_LIST_TTL` | `120s` | generic list responses |
| `PIPEDRIVE_CACHE_SEARCH_TTL` | `45s` | search endpoints |
| `PIPEDRIVE_CACHE_MAIL_LIST_TTL` | `60s` | `/mailbox/mailThreads` and `/deals/{id}/mailMessages` listings |
| `PIPEDRIVE_CACHE_MAIL_MESSAGE_TTL` | `24h` | `/mailbox/mailMessages/{id}` — bodies are immutable; mutable fields like `read_flag` can lag, bust with `pipedrive.cache.invalidate` |
| `PIPEDRIVE_CACHE_ALLOW_STALE_ON_429` | `true` | Serve expired cache if Pipedrive returns 429/5xx |
| `PIPEDRIVE_CACHE_STALE_TTL` | `24h` | Max age for stale fallback |

### HTTP header overrides (SSE / streamable-http)

| Header | Overrides |
|---|---|
| `X-Pipedrive-Domain` | `PIPEDRIVE_DOMAIN` |
| `X-Pipedrive-API-Token` | `PIPEDRIVE_API_TOKEN` |
| `X-Pipedrive-OAuth-Token` | `PIPEDRIVE_OAUTH_ACCESS_TOKEN` |

## Tools (68 total)

### Read

- `pipedrive.context.get` — users, pipelines, stages, deal-fields, activity-types (cached)
- `pipedrive.filters.list` — discover saved filters; pass the returned `id` as `filter_id` on any list tool to reproduce a Pipedrive UI view. See [docs/filters.md](docs/filters.md).
- `pipedrive.webhooks.list` — webhook subscriptions
- `pipedrive.deals.{list,get,search}`
- `pipedrive.deals.products.list`
- `pipedrive.persons.{list,get,search}`
- `pipedrive.organizations.{list,get,search}`
- `pipedrive.products.{list,get,search}`
- `pipedrive.leads.{list,get,search}`
- `pipedrive.activities.{list,get}`
- `pipedrive.notes.list`
- `pipedrive.deals.followers.list`
- `pipedrive.persons.followers.list`
- `pipedrive.organizations.followers.list`
- `pipedrive.mail.threads.{list,get}` — token owner's connected mailbox (single-user scope). `list` supports `since`, `unread_only`, `fields=compact|full`.
- `pipedrive.mail.messages.get` — one message with body. `body_format=text|html|none`. Cross-user scope.
- `pipedrive.deals.mail.list` — all mail tied to a deal across the workspace (cross-user; aggregates teammates' mail). Body backfill is bounded-concurrency and reuses the cache.
- `pipedrive.cache.stats`

### Write (gated by `PIPEDRIVE_ALLOW_WRITE=true`)

- `pipedrive.deals.{create,update,archive,unarchive}`
- `pipedrive.deal_fields.add_option` — append options to enum/set deal fields
- `pipedrive.deals.products.{attach,update,detach}`
- `pipedrive.persons.{create,update}`
- `pipedrive.organizations.{create,update}`
- `pipedrive.products.{create,update}`
- `pipedrive.filters.{create,update}` — saved filters for server-side (incl. custom-field) filtering
- `pipedrive.leads.{create,update}`
- `pipedrive.activities.{create,update,batch_create}` — batch_create supports up to 100 activities per call
- `pipedrive.notes.create`
- `pipedrive.deals.followers.{add,remove}`
- `pipedrive.persons.followers.{add,remove}`
- `pipedrive.organizations.followers.{add,remove}`

### Delete (gated by `PIPEDRIVE_ALLOW_WRITE=true` AND `PIPEDRIVE_ALLOW_DELETE=true`)

Delete tools require **both** flags — `PIPEDRIVE_ALLOW_DELETE=true` alone is not sufficient.

- `pipedrive.deals.delete`
- `pipedrive.persons.delete`
- `pipedrive.organizations.delete`
- `pipedrive.products.delete`
- `pipedrive.leads.delete`
- `pipedrive.activities.delete`
- `pipedrive.webhooks.delete`

### Admin (gated by `PIPEDRIVE_ENABLE_ADMIN_TOOLS=true`)

- `pipedrive.cache.clear`
- `pipedrive.cache.invalidate`

All write/delete tools are **registered** regardless of flag state; when disabled they return a structured error `{"error":"write_disabled"|"delete_disabled"|"admin_tools_disabled", "message":"..."}` so callers can detect capability without trial-and-error.

## Role-based configurations (token savings)

Every `tools/list` call ships the full schema of every registered tool into the LLM's context. The allowlist (`PIPEDRIVE_ALLOWED_TOOLS`) filters at **registration time**, so unlisted tools don't appear in `tools/list` at all — the token cost drops proportionally.

| Role | Tools | Tokens | Saved | Recipe |
|---|---:|---:|---:|---|
| admin | 63 | 8,952 | — | [docs/roles/admin.md](docs/roles/admin.md) |
| sales-manager | 47 | 7,987 | 10.8 % | [docs/roles/sales-manager.md](docs/roles/sales-manager.md) |
| sales-rep | 33 | 6,049 | 32.4 % | [docs/roles/sales-rep.md](docs/roles/sales-rep.md) |
| sdr | 24 | 4,498 | 49.8 % | [docs/roles/sdr.md](docs/roles/sdr.md) |
| read-only | 24 | 4,025 | 55.0 % | [docs/roles/read-only.md](docs/roles/read-only.md) |
| marketing | 22 | 4,279 | 52.2 % | [docs/roles/marketing.md](docs/roles/marketing.md) |
| customer-success | 19 | 3,623 | 59.5 % | [docs/roles/customer-success.md](docs/roles/customer-success.md) |

Measured with cl100k_base (OpenAI tokenizer). Claude's tokenizer is typically within ±10 %.

Token figures were measured against the pre-68-tool catalog; treat them as lower bounds until re-measured.

Each role file contains a ready-to-paste `PIPEDRIVE_ALLOWED_TOOLS` value plus the right `ALLOW_WRITE` / `ALLOW_DELETE` / `ENABLE_ADMIN_TOOLS` flags. Start at [docs/roles.md](docs/roles.md) for the index, Pipedrive permission-set mapping, and guidance on picking the right profile.

### Cache modes

Every read tool accepts an optional `cache_mode`:

| Mode | Behavior |
|---|---|
| `default` | Use non-expired cache; otherwise call API and store |
| `bypass` | Skip cache entirely for this call |
| `refresh` | Call API and update cache |
| `only` | Return cache miss error if nothing is cached |

Every response carries a `meta.cache` block:

```json
{
  "cache": {
    "hit": true,
    "stale": false,
    "stored_at": "2026-04-16T10:30:00Z",
    "expires_at": "2026-04-16T10:35:00Z",
    "ttl_seconds": 300
  }
}
```

## Running

### Docker

```bash
docker run -i --rm \
  -e PIPEDRIVE_API_TOKEN=xxx \
  -e PIPEDRIVE_DOMAIN=mycompany.pipedrive.com \
  luisra51/mcp-pipedrive:latest -t stdio
```

### Local

```bash
PIPEDRIVE_API_TOKEN=xxx PIPEDRIVE_DOMAIN=mycompany.pipedrive.com \
  go run ./cmd/mcp-pipedrive -t stdio
```

## Development

```bash
task dev:up                                 # start dev container
task dev:exec CMD='go build ./...'          # build
task dev:exec CMD='go test ./...'           # test
task dev:exec CMD='go run ./cmd/mcp-pipedrive -t stdio'
task dev:down                               # stop
```

## Release

Tag a semver commit to trigger the Docker Hub build (`luisra51/mcp-pipedrive`):

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT.
