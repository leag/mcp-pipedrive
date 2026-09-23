package tools

import (
	"fmt"

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
//
// The field map is only fetched once an item actually carries custom fields.
func decodeCustomFields[T any, PT interface {
	*T
	customFieldCarrier
}](client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode, items []T) {
	var fm *pipedrive.FieldMap
	fetched := false
	for i := range items {
		ptr := PT(&items[i]).CustomFieldsPtr()
		if len(*ptr) == 0 {
			continue
		}
		if !fetched {
			fm, fetched = fieldMapOrNil(client, entity, mode), true
		}
		*ptr = fm.DecodeCustomFields(*ptr)
	}
}

// decodeOneCustomFields is decodeCustomFields for a single entity.
func decodeOneCustomFields[T any, PT interface {
	*T
	customFieldCarrier
}](client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode, item *T) {
	ptr := PT(item).CustomFieldsPtr()
	if len(*ptr) == 0 {
		return
	}
	*ptr = fieldMapOrNil(client, entity, mode).DecodeCustomFields(*ptr)
}

// setCustomFields encodes caller-supplied custom fields to their wire form and
// attaches them to a write body. Unlike the read path this fails loudly: a name
// we cannot resolve would be silently ignored by Pipedrive, so the write would
// report success while dropping the value.
func setCustomFields(client *pipedrive.Client, entity pipedrive.FieldEntity, body map[string]any, cf map[string]any) error {
	if len(cf) == 0 {
		return nil
	}
	// On a metadata outage fm stays nil, which still lets raw hash keys through
	// so such callers are not blocked; names are rejected below.
	fm, metaErr := pipedrive.FetchFieldMap(client, entity, pipedrive.CacheModeDefault)
	encoded, err := fm.EncodeCustomFields(cf)
	if err != nil && metaErr == nil {
		// The cached metadata may predate a field or option added since;
		// re-fetch once before rejecting the write.
		if fresh, ferr := pipedrive.FetchFieldMap(client, entity, pipedrive.CacheModeRefresh); ferr == nil {
			encoded, err = fresh.EncodeCustomFields(cf)
		}
	}
	if err != nil {
		if metaErr != nil {
			return fmt.Errorf("custom field metadata unavailable (%v); pass 40-char field keys directly: %w", metaErr, err)
		}
		return err
	}
	body["custom_fields"] = encoded
	return nil
}

func fieldMapOrNil(client *pipedrive.Client, entity pipedrive.FieldEntity, mode pipedrive.CacheMode) *pipedrive.FieldMap {
	fm, err := pipedrive.FetchFieldMap(client, entity, mode)
	if err != nil {
		return nil
	}
	return fm
}
