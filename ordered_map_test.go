package json_repair

// Key-order regression tests (DECISIONS D-006): repairer output objects are
// *OrderedMap and must serialize with Python insertion-order keys
// (json.dumps of a Python dict), not sorted keys.
//
// Every expected string below was produced by the Python reference
// implementation (json_repair-main, .venv-oracle) on 2026-09-21 via
// repair_json(..., return_objects=True) + json.dumps(v, ensure_ascii=False)
// (no sort_keys). If a case stops matching, either the Go key order or the
// Python behavior changed — check the other side before editing expectations.

import (
	"encoding/json"
	"strings"
	"testing"
)

// orderedSchema builds an *OrderedMap from key/value pairs in the given
// order. A Go map literal would lose order and get sorted on normalization,
// so these tests need explicit property order (the whole point of D-006).
func orderedSchema(pairs ...any) *OrderedMap {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1])
	}
	return m
}

// keyOrderDump runs RepairJSON (object form) and serializes with Dump's
// Python-default separators, the exact form compared against
// json.dumps(..., ensure_ascii=False).
func keyOrderDump(t *testing.T, s string, opts Options) string {
	t.Helper()
	opts.ReturnObjects = true
	v, _, err := RepairJSON(s, opts)
	if err != nil {
		t.Fatalf("RepairJSON(%q): %v", s, err)
	}
	out, err := Dump(v, DumpOptions{EnsureASCII: false})
	if err != nil {
		t.Fatalf("Dump(%q): %v", s, err)
	}
	return out
}

func TestOrderedMapSemantics(t *testing.T) {
	m := NewOrderedMap()
	m.Set("z", int64(1))
	m.Set("a", int64(2))
	m.Set("z", int64(3)) // existing key: value updated in place, position kept
	if got := m.Keys(); strings.Join(got, ",") != "z,a" {
		t.Fatalf("Keys after in-place update = %v, want [z a]", got)
	}
	if v, ok := m.Get("z"); !ok || v != int64(3) {
		t.Fatalf("Get(z) = (%v, %v), want (3, true)", v, ok)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}

	m.Delete("z")
	if m.Has("z") || m.Len() != 1 {
		t.Fatalf("after Delete: Has(z)=%v Len=%d, want false/1", m.Has("z"), m.Len())
	}
	if v := m.GetOrNil("z"); v != nil {
		t.Fatalf("GetOrNil(z) = %v, want nil", v)
	}

	m.Set("m", int64(4)) // re-added key appends at the end
	if got := m.Keys(); strings.Join(got, ",") != "a,m" {
		t.Fatalf("Keys after re-add = %v, want [a m]", got)
	}
	if vs := m.Values(); len(vs) != 2 || vs[0] != int64(2) || vs[1] != int64(4) {
		t.Fatalf("Values = %v, want [2 4]", vs)
	}

	c := m.Clone()
	c.Set("z", int64(9)) // clone is independent
	if m.Has("z") {
		t.Fatalf("Clone leaked mutation back into original")
	}
	if got := c.Keys(); strings.Join(got, ",") != "a,m,z" {
		t.Fatalf("Clone Keys = %v, want [a m z]", got)
	}

	plain := m.ToPlain()
	if len(plain) != 2 || plain["a"] != int64(2) || plain["m"] != int64(4) {
		t.Fatalf("ToPlain = %#v", plain)
	}
}

func TestKeyOrderFastPath(t *testing.T) {
	// Valid JSON fast path: keys keep document (insertion) order.
	cases := []struct {
		in   string
		want string
	}{
		{`{"z": 1, "a": 2, "m": 3}`, `{"z": 1, "a": 2, "m": 3}`},
		{`{"z": {"a": 1}, "m": [1, {"z": 1, "a": 2}]}`, `{"z": {"a": 1}, "m": [1, {"z": 1, "a": 2}]}`},
	}
	for _, c := range cases {
		if got := keyOrderDump(t, c.in, Options{}); got != c.want {
			t.Errorf("in=%s\n got=%s\nwant=%s", c.in, got, c.want)
		}
	}
}

func TestKeyOrderRepairPath(t *testing.T) {
	// Repairer path (skip_json_loads): same insertion-order semantics.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"truncated", `{"b": 2, "a": 1`, `{"b": 2, "a": 1}`},
		{"unquoted", `{b: 2, a: 1`, `{"b": 2, "a": 1}`},
		// Duplicate key: value updated in place, first position kept.
		{"dup_key", `{"b": 1, "a": 2, "b": 3}`, `{"b": 3, "a": 2}`},
		// Trailing merge into the previous object.
		{"trailing_merge", `{"b": 1}, "a": 2}`, `{"b": 1, "a": 2}`},
		// Two top-level objects of different shape -> array, element order kept.
		{"two_top", `{"a": 1} {"b": 2}`, `[{"a": 1}, {"b": 2}]`},
		// Two top-level objects of the same shape -> merged in place.
		{"two_top_same", `{"a": 1} {"a": 2}`, `{"a": 2}`},
	}
	for _, c := range cases {
		if got := keyOrderDump(t, c.in, Options{SkipJsonLoads: true}); got != c.want {
			t.Errorf("%s: in=%s\n got=%s\nwant=%s", c.name, c.in, got, c.want)
		}
	}
}

func TestKeyOrderSchemaRebuild(t *testing.T) {
	// Schema-guided repair rebuilds objects in schema property order, then
	// appends the remaining input keys in input order (Python _repair_object).
	schema := orderedSchema(
		"type", "object",
		"properties", orderedSchema(
			"zeta", orderedSchema("type", "string"),
			"alpha", orderedSchema("type", "string"),
		),
		"additionalProperties", true,
	)
	if got := keyOrderDump(t, `{"alpha": "a", "zeta": "z"`,
		Options{SkipJsonLoads: true, Schema: schema}); got != `{"zeta": "z", "alpha": "a"}` {
		t.Errorf("schema rebuild got=%s", got)
	}
	// Extra key (not in properties) appends after the schema properties, in
	// input order.
	if got := keyOrderDump(t, `{"extra": 1, "alpha": "a"`,
		Options{SkipJsonLoads: true, Schema: schema}); got != `{"alpha": "a", "extra": 1}` {
		t.Errorf("schema extra got=%s", got)
	}

	// finalizeObject fills the missing defaulted property, then the repair
	// rebuild puts it in schema order.
	schemaDefault := orderedSchema(
		"type", "object",
		"properties", orderedSchema(
			"zeta", orderedSchema("type", "string", "default", "DZ"),
			"alpha", orderedSchema("type", "string"),
		),
	)
	if got := keyOrderDump(t, `{"alpha": "a"`,
		Options{SkipJsonLoads: true, Schema: schemaDefault}); got != `{"zeta": "DZ", "alpha": "a"}` {
		t.Errorf("schema default got=%s", got)
	}
}

func TestKeyOrderSalvageListToObject(t *testing.T) {
	// Salvage _map_list_to_object maps array items onto schema properties in
	// schema property order (Python dict insertion order).
	schema := orderedSchema(
		"type", "object",
		"properties", orderedSchema(
			"z", orderedSchema("type", "integer"),
			"a", orderedSchema("type", "integer"),
		),
		"required", []any{"z", "a"},
	)
	mode := "salvage"
	if got := keyOrderDump(t, `[1, 2]`,
		Options{SkipJsonLoads: true, Schema: schema, SchemaRepairMode: mode}); got != `{"z": 1, "a": 2}` {
		t.Errorf("salvage list->object got=%s", got)
	}
}

func TestKeyOrderObjectReturnForm(t *testing.T) {
	// ReturnObjects form: the returned tree is *OrderedMap with document
	// key order, so a user re-marshal (MarshalJSON contract) keeps it.
	v, _, err := RepairJSON(`{"z": 1, "a": {"q": 1, "b": 2}}`, Options{ReturnObjects: true})
	if err != nil {
		t.Fatalf("RepairJSON: %v", err)
	}
	om, ok := v.(*OrderedMap)
	if !ok {
		t.Fatalf("top level = %T, want *OrderedMap", v)
	}
	if got := om.Keys(); strings.Join(got, ",") != "z,a" {
		t.Fatalf("top-level Keys = %v, want [z a]", got)
	}
	inner, ok := om.GetOrNil("a").(*OrderedMap)
	if !ok {
		t.Fatalf("a = %T, want *OrderedMap", om.GetOrNil("a"))
	}
	if got := inner.Keys(); strings.Join(got, ",") != "q,b" {
		t.Fatalf("inner Keys = %v, want [q b]", got)
	}
	// MarshalJSON contract: encoding/json consumers emit Python order too.
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalJSON: %v", err)
	}
	if string(b) != `{"z":1,"a":{"q":1,"b":2}}` {
		t.Fatalf("MarshalJSON = %s", b)
	}
}
