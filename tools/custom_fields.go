package tools

import (
	"context"

	"mcp-pipedrive/pipedrive"
)

// customFieldCarrier is any normalized entity that can hold custom fields.
type customFieldCarrier interface {
	CustomFieldsPtr() *map[string]any
}

// decodeCustomFields rewrites the custom_fields map of each item in place:
// nulls dropped, hash keys replaced by field names, option IDs by their labels.
//
// A metadata failure is deliberately not fatal. The field map is a readability
// aid; if it cannot be loaded we still drop nulls (the bulk of the savings) and
// return hash keys, rather than failing a read the caller could otherwise use.
func decodeCustomFields[T any, PT interface {
	*T
	customFieldCarrier
}](ctx context.Context, client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode, items []T) {
	if len(items) == 0 {
		return
	}
	fm := fieldMapOrNil(ctx, client, entity, mode)
	for i := range items {
		ptr := PT(&items[i]).CustomFieldsPtr()
		*ptr = fm.DecodeCustomFields(*ptr)
	}
}

// decodeOneCustomFields is decodeCustomFields for a single entity.
func decodeOneCustomFields[T any, PT interface {
	*T
	customFieldCarrier
}](ctx context.Context, client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode, item *T) {
	fm := fieldMapOrNil(ctx, client, entity, mode)
	ptr := PT(item).CustomFieldsPtr()
	*ptr = fm.DecodeCustomFields(*ptr)
}

// setCustomFields encodes caller-supplied custom fields to their wire form and
// attaches them to a write body. Unlike the read path this fails loudly: a name
// we cannot resolve would be silently ignored by Pipedrive, so the write would
// report success while dropping the value.
func setCustomFields(ctx context.Context, client *pipedrive.Client, entity pipedrive.FieldEntity, body map[string]any, cf map[string]any) error {
	if len(cf) == 0 {
		return nil
	}
	fm, err := pipedrive.FetchFieldMap(client, entity, pipedrive.CacheModeDefault)
	if err != nil {
		// Metadata unavailable: pass through unchanged so a caller using raw
		// hash keys is not blocked by a metadata outage.
		body["custom_fields"] = cf
		return nil
	}
	encoded, err := fm.EncodeCustomFields(cf)
	if err != nil {
		return err
	}
	body["custom_fields"] = encoded
	return nil
}

func fieldMapOrNil(ctx context.Context, client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode) *pipedrive.FieldMap {
	fm, err := pipedrive.FetchFieldMap(client, entity, mode)
	if err != nil {
		return nil
	}
	return fm
}
