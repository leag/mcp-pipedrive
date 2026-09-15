# Custom fields

Pipedrive returns custom fields keyed by a 40-character hash, with `enum` and
`set` values as numeric option IDs, and it includes every field on every record
whether or not it is set. On a real account that is the single largest source of
useless context in a response: a deal carries ~82 custom fields of which ~95%
are `null`, so a default `deals.list` (limit 50) is ~275 KB, almost all of it
padding.

This server translates custom fields in both directions.

## Reads

`list` / `get` / `search` and the entity returned by a write all pass their
`custom_fields` object through a decode step:

1. **Null and empty values are dropped.** They carry no information.
2. **Hash keys become field names** — `aaaa1111…` → `# of devices`.
3. **Option IDs become labels** — `"Prey plan": 53` → `"Prey plan": "full_suite"`.
   A `set` field decodes to an array of labels.

```jsonc
// wire — 82 keys, ~5.2 KB
"custom_fields": {"0039f3b2…": null, "684fd03f…": 53, "d128753c…": 11, /* …79 more… */}

// decoded — 2 keys, ~60 B
"custom_fields": {"Prey plan": "full_suite", "# of devices": 11}
```

Anything that cannot be resolved is passed through untouched rather than
dropped: an unknown hash key keeps its hash, and an option ID that is not in the
metadata keeps its number. Losing a value the caller asked for would be worse
than an opaque key.

To see the raw payload, pass `include_raw: true` — it is unchanged by any of
this.

## Writes

`create` / `update` accept **either** form for `custom_fields`:

```jsonc
{"custom_fields": {"Prey plan": "full_suite", "# of devices": 11}}   // names + labels
{"custom_fields": {"684fd03f…": 53, "d128753c…": 11}}                 // keys + option IDs
```

Both produce the same request. This is what makes the translation safe: a model
can copy what a read returned straight into an update.

Unlike reads, an unresolvable name or option label is a **hard error**. Pipedrive
accepts unknown custom-field keys and silently ignores them, so passing one
through would report a successful write that quietly dropped the value. The
error lists the valid option labels for the field.

## Duplicate field names

Pipedrive permits two custom fields to share a name. When that happens, *every*
field in the colliding group is emitted as `Name (12345678)` using the first 8
characters of its key, so emitted names stay unique and reversible. Fields with
a unique name are never suffixed.

## Metadata and caching

The translation table comes from `/v1/dealFields`, `/v1/personFields`,
`/v1/organizationFields` and `/v1/productFields` (leads share the deal schema).
These go through the normal GET cache at the **metadata TTL** (12 h by default,
`PIPEDRIVE_CACHE_METADATA_TTL`), so after the first call the per-request cost is
a local bbolt read. A caller's `cache_mode=bypass` applies to the entity request
but *not* to this metadata lookup, which would otherwise add a network round
trip to every call.

If the metadata cannot be fetched, reads degrade gracefully: nulls are still
dropped (the bulk of the savings) and keys stay as hashes. Writes pass the
caller's `custom_fields` through unchanged, so a caller using raw hash keys is
not blocked by a metadata outage.
