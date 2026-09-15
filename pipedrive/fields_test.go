package pipedrive

import (
	"reflect"
	"testing"
)

// fieldsPayload mimics the `data` array of GET /v1/dealFields.
func fieldsPayload() []any {
	return []any{
		map[string]any{"key": "title", "name": "Title", "field_type": "varchar"},
		map[string]any{"key": "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", "name": "# of devices", "field_type": "double"},
		map[string]any{"key": "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222", "name": "Prey plan", "field_type": "enum",
			"options": []any{
				map[string]any{"id": float64(51), "label": "tracking"},
				map[string]any{"id": float64(53), "label": "full_suite"},
			}},
		map[string]any{"key": "cccc3333cccc3333cccc3333cccc3333cccc3333", "name": "Regions", "field_type": "set",
			"options": []any{
				map[string]any{"id": float64(1), "label": "LATAM"},
				map[string]any{"id": float64(2), "label": "EMEA"},
			}},
	}
}

func TestBuildFieldMapSkipsBaseFields(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	if _, ok := fm.byHash["title"]; ok {
		t.Fatal("base field `title` must not be treated as a custom field")
	}
	if len(fm.byHash) != 3 {
		t.Fatalf("want 3 custom fields, got %d", len(fm.byHash))
	}
}

func TestDecodeDropsNullsAndRenames(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	got := fm.DecodeCustomFields(map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": float64(11),
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": float64(53),
		"dddd4444dddd4444dddd4444dddd4444dddd4444": nil, // unknown AND null
	})
	want := map[string]any{"# of devices": float64(11), "Prey plan": "full_suite"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDecodeSetFieldToLabels(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	got := fm.DecodeCustomFields(map[string]any{
		"cccc3333cccc3333cccc3333cccc3333cccc3333": []any{float64(2), float64(1)},
	})
	want := map[string]any{"Regions": []any{"EMEA", "LATAM"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDecodeKeepsUnknownHashAndUnknownOption(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	got := fm.DecodeCustomFields(map[string]any{
		"dddd4444dddd4444dddd4444dddd4444dddd4444": "kept",
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": float64(99), // option not in metadata
	})
	want := map[string]any{
		"dddd4444dddd4444dddd4444dddd4444dddd4444": "kept",
		"Prey plan": float64(99),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestEncodeAcceptsNamesAndLabels(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	got, err := fm.EncodeCustomFields(map[string]any{
		"# of devices": float64(11),
		"Prey plan":    "full_suite",
		"Regions":      []any{"EMEA"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": float64(11),
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": int64(53),
		"cccc3333cccc3333cccc3333cccc3333cccc3333": []any{int64(2)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

// Round-tripping what a read returned must produce the original wire payload —
// this is the whole point of symmetric translation.
func TestEncodeIsInverseOfDecode(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	wire := map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": int64(11),
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": int64(53),
	}
	decoded := fm.DecodeCustomFields(map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": float64(11),
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": float64(53),
	})
	got, err := fm.EncodeCustomFields(decoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"] != wire["bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"] {
		t.Fatalf("enum did not round-trip: %#v", got)
	}
}

func TestEncodePassesThroughHashKeys(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	got, err := fm.EncodeCustomFields(map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": float64(11),
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": float64(53),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"] != float64(11) {
		t.Fatalf("hash key not passed through: %#v", got)
	}
}

func TestEncodeRejectsUnknownName(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	_, err := fm.EncodeCustomFields(map[string]any{"Nope": 1})
	if err == nil {
		t.Fatal("want error for unresolvable field name")
	}
}

func TestEncodeRejectsUnknownOptionLabel(t *testing.T) {
	fm := BuildFieldMap(fieldsPayload())
	_, err := fm.EncodeCustomFields(map[string]any{"Prey plan": "nonexistent"})
	if err == nil {
		t.Fatal("want error for unresolvable option label")
	}
}

// Pipedrive allows duplicate field names; the emitted name must stay unique and
// deterministic so encode can invert it.
func TestDuplicateNamesAreDisambiguated(t *testing.T) {
	fm := BuildFieldMap([]any{
		map[string]any{"key": "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", "name": "Notes", "field_type": "varchar"},
		map[string]any{"key": "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222", "name": "Notes", "field_type": "varchar"},
	})
	got := fm.DecodeCustomFields(map[string]any{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111": "x",
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222": "y",
	})
	want := map[string]any{"Notes (aaaa1111)": "x", "Notes (bbbb2222)": "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	back, err := fm.EncodeCustomFields(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if back["aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"] != "x" || back["bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"] != "y" {
		t.Fatalf("disambiguated names did not round-trip: %#v", back)
	}
}

// A field name that looks like a hash must not shadow a real hash key.
func TestNilFieldMapIsSafe(t *testing.T) {
	var fm *FieldMap
	got := fm.DecodeCustomFields(map[string]any{"a": nil, "b": 1})
	want := map[string]any{"b": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nil map must still drop nulls: got %#v want %#v", got, want)
	}
	back, err := fm.EncodeCustomFields(map[string]any{"b": 1})
	if err != nil || !reflect.DeepEqual(back, map[string]any{"b": 1}) {
		t.Fatalf("nil map encode must pass through: %#v %v", back, err)
	}
}
