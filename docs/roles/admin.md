# Role: admin

**Full access.** Equivalent to Pipedrive's built-in admin user type — unrestricted `deals:full` + `contacts:full` + `products:full` + `activities:full` + `leads:full` + `mail:full` + the destructive cache admin tools.

- **Tools exposed:** 68 (every tool the MCP ships)
- **Tokens (tools/list):** 8,952 (measured against the pre-68-tool catalog; treat as a lower bound until re-measured)
- **Savings vs baseline:** 0 (this is the baseline)

## Who should use it

Account owners, CRM administrators, and integrations that perform data migrations or mass cleanup. Also the right choice for a local developer exploring the full surface.

## Flags

```env
PIPEDRIVE_ALLOW_WRITE=true
PIPEDRIVE_ALLOW_DELETE=true
PIPEDRIVE_ENABLE_ADMIN_TOOLS=true
# PIPEDRIVE_ALLOWED_TOOLS intentionally unset — registers every tool
```

No allowlist is the point: admin needs everything. Setting `PIPEDRIVE_ALLOWED_TOOLS` would only subtract.

## What is exposed

- `pipedrive.context.get`
- `pipedrive.deals.{list,get,search,create,update,delete,archive,unarchive}`
- `pipedrive.deal_fields.add_option`
- `pipedrive.deals.products.{list,attach,update,detach}`
- `pipedrive.deals.followers.{list,add,remove}`
- `pipedrive.persons.{list,get,search,create,update,delete}`
- `pipedrive.persons.followers.{list,add,remove}`
- `pipedrive.organizations.{list,get,search,create,update,delete}`
- `pipedrive.organizations.followers.{list,add,remove}`
- `pipedrive.products.{list,get,search,create,update,delete}`
- `pipedrive.filters.{list,create,update}`
- `pipedrive.leads.{list,get,search,create,update,delete}`
- `pipedrive.activities.{list,get,create,update,delete,batch_create}`
- `pipedrive.notes.{list,create}`
- `pipedrive.mail.threads.{list,get}`
- `pipedrive.mail.messages.get`
- `pipedrive.deals.mail.list`
- `pipedrive.webhooks.{list,create,delete}`
- `pipedrive.cache.{stats,clear,invalidate}`

## Safety reminders

- Admin tools `cache.clear` / `cache.invalidate` are destructive against the local bbolt file — they don't touch Pipedrive itself, but they do drop all cached responses (future reads hit the API until re-populated).
- This role bypasses the token-saving rationale. If the caller only needs a subset, pick a narrower role instead.
