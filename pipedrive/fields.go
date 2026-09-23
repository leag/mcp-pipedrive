package pipedrive

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// hashKeyRe matches Pipedrive's custom-field keys, which are 40-char hex
// digests. Base fields (title, value, status, ...) use plain names, so this is
// what separates the two in a *Fields payload.
var hashKeyRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// FieldEntity selects which *Fields metadata endpoint backs a FieldMap.
type FieldEntity string

const (
	FieldEntityDeal         FieldEntity = "deal"
	FieldEntityPerson       FieldEntity = "person"
	FieldEntityOrganization FieldEntity = "organization"
	FieldEntityProduct      FieldEntity = "product"
)

// fieldsPath maps an entity to its v1 metadata endpoint. Leads share the deal
// custom-field schema, so they reuse FieldEntityDeal.
var fieldsPath = map[FieldEntity]string{
	FieldEntityDeal:         "/dealFields",
	FieldEntityPerson:       "/personFields",
	FieldEntityOrganization: "/organizationFields",
	FieldEntityProduct:      "/productFields",
}

// FieldDef is one custom field, with its option table flattened for lookup in
// both directions.
type FieldDef struct {
	Key        string // 40-char hash, the wire key
	Name       string // human name as emitted, already disambiguated
	Type       string // field_type; "set" values are always sent as arrays
	optByID    map[int64]string
	optByLabel map[string]int64
}

func (f *FieldDef) hasOptions() bool { return len(f.optByID) > 0 }

// FieldMap translates a custom-field payload between Pipedrive's wire form
// (hash keys, numeric option IDs) and a human form (field names, option
// labels). It is built from cached *Fields metadata and is read-only once
// built, so it is safe to share across goroutines.
type FieldMap struct {
	byHash map[string]*FieldDef
	byName map[string]*FieldDef
}

// BuildFieldMap indexes the `data` array of a *Fields response. Base
// (non-custom) fields are ignored: they never appear in a custom_fields object.
//
// Pipedrive permits two custom fields to share a name. When that happens every
// field in the colliding group is emitted as "Name (12345678)" using the first
// 8 chars of its hash, so the emitted name stays unique and EncodeCustomFields
// can invert it.
func BuildFieldMap(raw []any) *FieldMap {
	fm := &FieldMap{byHash: map[string]*FieldDef{}, byName: map[string]*FieldDef{}}

	byName := map[string][]*FieldDef{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := toString(m["key"])
		if !hashKeyRe.MatchString(key) {
			continue
		}
		def := &FieldDef{Key: key, Name: toString(m["name"]), Type: toString(m["field_type"])}
		if def.Name == "" {
			def.Name = key
		}
		if opts, ok := m["options"].([]any); ok {
			for _, o := range opts {
				om, ok := o.(map[string]any)
				if !ok {
					continue
				}
				label := toString(om["label"])
				if label == "" {
					continue
				}
				id := toInt64(om["id"])
				if def.optByID == nil {
					def.optByID = map[int64]string{}
					def.optByLabel = map[string]int64{}
				}
				def.optByID[id] = label
				def.optByLabel[label] = id
			}
		}
		fm.byHash[key] = def
		byName[def.Name] = append(byName[def.Name], def)
	}

	for name, defs := range byName {
		if len(defs) == 1 {
			fm.byName[name] = defs[0]
			continue
		}
		for _, d := range defs {
			d.Name = fmt.Sprintf("%s (%s)", name, d.Key[:8])
			fm.byName[d.Name] = d
		}
	}
	return fm
}

// DecodeCustomFields turns a wire custom_fields object into its human form:
// null-valued fields are dropped entirely, hash keys become field names, and
// enum/set option IDs become their labels.
//
// Anything it cannot resolve is passed through untouched rather than dropped —
// losing a value the caller asked for would be worse than an opaque key. A nil
// FieldMap (metadata unavailable) still drops nulls.
func (fm *FieldMap) DecodeCustomFields(cf map[string]any) map[string]any {
	if len(cf) == 0 {
		return nil
	}
	out := make(map[string]any, len(cf))
	for k, v := range cf {
		if isEmptyValue(v) {
			continue
		}
		if fm == nil {
			out[k] = v
			continue
		}
		def, ok := fm.byHash[k]
		if !ok {
			out[k] = v
			continue
		}
		out[def.Name] = decodeValue(def, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EncodeCustomFields is the inverse of DecodeCustomFields: it accepts either
// field names or raw hash keys, and either option labels or raw option IDs, and
// returns the wire form.
//
// Unlike decode, an unresolvable key or label is an error. Passing it through
// would send Pipedrive a field it silently ignores, so the write would report
// success while dropping data. A nil FieldMap (metadata unavailable) therefore
// accepts only raw hash keys, passed through unchanged.
func (fm *FieldMap) EncodeCustomFields(cf map[string]any) (map[string]any, error) {
	if len(cf) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(cf))
	var unknown []string
	for k, v := range cf {
		var def *FieldDef
		ok := false
		if fm != nil {
			def, ok = fm.byHash[k]
			if !ok {
				def, ok = fm.byName[k]
			}
		}
		if !ok {
			// A 40-char hash we have no metadata for is still a plausible key
			// (the field may be newer than the cached metadata); a free-form
			// name is not.
			if hashKeyRe.MatchString(k) {
				out[k] = v
				continue
			}
			unknown = append(unknown, k)
			continue
		}
		encoded, err := encodeValue(def, v)
		if err != nil {
			unknown = append(unknown, fmt.Sprintf("%s: %v", k, err))
			continue
		}
		out[def.Key] = encoded
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("unresolved custom fields (%s) — list the entity once to see the available field names, or pass the 40-char field key directly", strings.Join(unknown, "; "))
	}
	return out, nil
}

func decodeValue(def *FieldDef, v any) any {
	if !def.hasOptions() {
		return v
	}
	switch t := v.(type) {
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, decodeOption(def, e))
		}
		return out
	case string:
		// A `set` can arrive as a comma-joined ID list in v1 payloads.
		if strings.Contains(t, ",") {
			parts := strings.Split(t, ",")
			out := make([]any, 0, len(parts))
			for _, p := range parts {
				out = append(out, decodeOption(def, strings.TrimSpace(p)))
			}
			return out
		}
		return decodeOption(def, t)
	default:
		return decodeOption(def, v)
	}
}

func decodeOption(def *FieldDef, v any) any {
	var id int64
	switch t := v.(type) {
	case string:
		// Already a label? Leave it alone.
		if _, ok := def.optByLabel[t]; ok {
			return t
		}
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return v
		}
		id = n
	default:
		id = toInt64(v)
	}
	if label, ok := def.optByID[id]; ok {
		return label
	}
	return v
}

func encodeValue(def *FieldDef, v any) (any, error) {
	// null clears the field, whatever its type.
	if v == nil || !def.hasOptions() {
		return v, nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			id, err := encodeOption(def, e)
			if err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, nil
	default:
		id, err := encodeOption(def, v)
		if err != nil {
			return nil, err
		}
		if def.Type == "set" {
			return []any{id}, nil
		}
		return id, nil
	}
}

func encodeOption(def *FieldDef, v any) (any, error) {
	if s, ok := v.(string); ok {
		if id, ok := def.optByLabel[s]; ok {
			return id, nil
		}
		// Callers may pass the numeric ID as a string.
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if _, known := def.optByID[n]; known {
				return n, nil
			}
		}
		return nil, fmt.Errorf("unknown option %q (valid: %s)", s, strings.Join(optionLabels(def), ", "))
	}
	id := toInt64(v)
	if _, ok := def.optByID[id]; ok {
		return id, nil
	}
	return nil, fmt.Errorf("unknown option id %v (valid: %s)", v, strings.Join(optionLabels(def), ", "))
}

func optionLabels(def *FieldDef) []string {
	out := make([]string, 0, len(def.optByLabel))
	for l := range def.optByLabel {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// isEmptyValue reports values that carry no information and only cost context:
// nulls, and the empty arrays Pipedrive returns for unset multi-option fields.
func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// FetchFieldMap loads and indexes the custom-field metadata for an entity. It
// goes through the normal GET cache at the metadata TTL, so the per-call cost
// after the first is a local bbolt read.
func FetchFieldMap(client *Client, entity FieldEntity, mode CacheMode) (*FieldMap, error) {
	path, ok := fieldsPath[entity]
	if !ok {
		return nil, fmt.Errorf("unknown field entity %q", entity)
	}
	req, err := client.NewRequest(V1, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	// Metadata is looked up on nearly every call; never let a caller's
	// `bypass` turn that into a second network round trip per request.
	if mode == CacheModeBypass {
		mode = CacheModeDefault
	}
	ck := Key(client, V1, http.MethodGet, path, url.Values{}, nil)
	payload, _, err := client.CachedGet(req, ck, client.TTLs.Metadata, mode)
	if err != nil {
		return nil, err
	}
	m, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected %s payload", path)
	}
	arr, _ := m["data"].([]any)
	return BuildFieldMap(arr), nil
}

// CustomFieldsPtr exposes the custom-field map of each normalized entity so a
// single decode helper can walk any of them.
func (d *NormalizedDeal) CustomFieldsPtr() *map[string]any         { return &d.CustomFields }
func (p *NormalizedPerson) CustomFieldsPtr() *map[string]any       { return &p.CustomFields }
func (o *NormalizedOrganization) CustomFieldsPtr() *map[string]any { return &o.CustomFields }
func (p *NormalizedProduct) CustomFieldsPtr() *map[string]any      { return &p.CustomFields }
func (l *NormalizedLead) CustomFieldsPtr() *map[string]any         { return &l.CustomFields }
