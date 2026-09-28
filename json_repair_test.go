package json_repair

import (
	"encoding/json"
	"reflect"
	"testing"
)

// rep repairs s (object form, i.e. the Python return_objects=True result).
func rep(t *testing.T, s string, skip bool) any {
	t.Helper()
	v, _, err := RepairJSON(s, Options{SkipJsonLoads: skip})
	if err != nil {
		t.Fatalf("RepairJSON(%q): %v", s, err)
	}
	return v
}

func repWithLog(t *testing.T, s string, skip bool) (any, []RepairLog) {
	t.Helper()
	v, logs, err := RepairJSON(s, Options{SkipJsonLoads: skip, Logging: true})
	if err != nil {
		t.Fatalf("RepairJSON(%q): %v", s, err)
	}
	return v, logs
}

func assertEq(t *testing.T, got, want any) {
	t.Helper()
	// Compare through plain maps: repairer output objects are *OrderedMap
	// (insertion order, D-006) while test expectations are orderless map
	// literals — Python dict == is also key-order insensitive.
	if !reflect.DeepEqual(toPlainValue(got), toPlainValue(want)) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// assertRepair compares object-form repair output against a JSON `want`
// string. Used by the generated parity tests (tools/gen_parity_test.py).
func assertRepair(t *testing.T, in, want string, skip, strict bool) {
	t.Helper()
	got, _, err := RepairJSON(in, Options{SkipJsonLoads: skip, Strict: strict})
	if err != nil {
		t.Fatalf("RepairJSON(%q): %v", in, err)
	}
	var wantVal any
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("bad want JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(normNum(toPlainValue(got)), normNum(wantVal)) {
		t.Fatalf("input %q\n got  %#v\n want %#v", in, got, wantVal)
	}
}

func TestParseNumber(t *testing.T) {
	assertEq(t, rep(t, "1", false), int64(1))
	assertEq(t, rep(t, "1.2", false), 1.2)
	assertEq(t, rep(t, `{"value": 82_461_110}`, false), map[string]any{"value": int64(82461110)})
	assertEq(t, rep(t, `{"value": 1_234.5_6}`, false), map[string]any{"value": 1234.56})
}

func TestParseNumberEdgeCases(t *testing.T) {
	assertEq(t, rep(t, ` - { "test_key": ["test_value", "test_value2"] }`, false),
		map[string]any{"test_key": []any{"test_value", "test_value2"}})
	assertEq(t, rep(t, `{"key": 1/3}`, false), map[string]any{"key": "1/3"})
	assertEq(t, rep(t, `{"key": .25}`, false), map[string]any{"key": 0.25})
	assertEq(t, rep(t, `{"here": "now", "key": 1/3, "foo": "bar"}`, false),
		map[string]any{"here": "now", "key": "1/3", "foo": "bar"})
	assertEq(t, rep(t, `{"key": 12345/67890}`, false), map[string]any{"key": "12345/67890"})
	assertEq(t, rep(t, "[105,12", false), []any{int64(105), int64(12)})
	assertEq(t, rep(t, `{"key", 105,12,}`, false), map[string]any{"key": "105,12"})
	assertEq(t, rep(t, `{"key": 1/3, "foo": "bar"}`, false), map[string]any{"key": "1/3", "foo": "bar"})
	assertEq(t, rep(t, `{"key": 10-20}`, false), map[string]any{"key": "10-20"})
	assertEq(t, rep(t, `{"key": 1.1.1}`, false), map[string]any{"key": "1.1.1"})
	assertEq(t, rep(t, "[- ", false), []any{})
	assertEq(t, rep(t, `{"key": 1. }`, false), map[string]any{"key": 1.0})
	assertEq(t, rep(t, `{"key": 1e10 }`, false), map[string]any{"key": 10000000000.0})
	assertEq(t, rep(t, `{"key": 1e }`, false), map[string]any{"key": int64(1)})
	assertEq(t, rep(t, `{"key": 1notanumber }`, false), map[string]any{"key": "1notanumber"})
	assertEq(t, rep(t, `{"rowId": 57eeeeb1-450b-482c-81b9-4be77e95dee2}`, false),
		map[string]any{"rowId": "57eeeeb1-450b-482c-81b9-4be77e95dee2"})
	assertEq(t, rep(t, "[1, 2notanumber]", false), []any{int64(1), "2notanumber"})
}

func TestParseComment(t *testing.T) {
	assertEq(t, rep(t, "/", false), "")
	assertEq(t, rep(t, `/* comment */ {"key": "value"}`, false), map[string]any{"key": "value"})
	assertEq(t, rep(t, `{ "key": { "key2": "value2" // comment }, "key3": "value3" }`, false),
		map[string]any{"key": map[string]any{"key2": "value2"}})
	assertEq(t, rep(t, "{ \"key\": { \"key2\": \"value2\" // comment\n}, \"key3\": \"value3\" }", false),
		map[string]any{"key": map[string]any{"key2": "value2"}, "key3": "value3"})
	assertEq(t, rep(t, `{ "key": { "key2": "value2" # comment }, "key3": "value3" }`, false),
		map[string]any{"key": map[string]any{"key2": "value2"}, "key3": "value3"})
	assertEq(t, rep(t, `{ "key": { "key2": "value2" /* comment */ }, "key3": "value3" }`, false),
		map[string]any{"key": map[string]any{"key2": "value2"}, "key3": "value3"})
	assertEq(t, rep(t, `[ "value", /* comment */ "value2" ]`, false), []any{"value", "value2"})
	assertEq(t, rep(t, `{ "key": "value" /* comment`, false), map[string]any{"key": "value"})
}

func TestManyTopLevelCommentsWithoutRecursionError(t *testing.T) {
	raw := make([]string, 0, 601)
	for i := 0; i < 600; i++ {
		raw = append(raw, "# comment\n")
	}
	raw = append(raw, `{"key": "value"}`)
	s := string([]byte(joinStrings(raw)))

	v, logs := repWithLog(t, s, true)
	assertEq(t, v, map[string]any{"key": "value"})
	if len(logs) != 600 {
		t.Fatalf("expected 600 logs, got %d", len(logs))
	}
	for _, log := range logs {
		if log.Text != "Found line comment: # comment, ignoring" {
			t.Fatalf("unexpected log text: %q", log.Text)
		}
	}
}

func TestValidJSONFastPath(t *testing.T) {
	assertEq(t, rep(t, `{"name": "John", "age": 30, "city": "New York"}`, false),
		map[string]any{"name": "John", "age": int64(30), "city": "New York"})
	assertEq(t, rep(t, `{"key": "value:value"}`, false), map[string]any{"key": "value:value"})
	assertEq(t, rep(t, `{"key1": {"key2": [1, 2, 3]}}`, false),
		map[string]any{"key1": map[string]any{"key2": []any{int64(1), int64(2), int64(3)}}})
}

func TestBasicObjectRepair(t *testing.T) {
	assertEq(t, rep(t, `{`, false), map[string]any{})
	assertEq(t, rep(t, `{"`, false), map[string]any{})
	assertEq(t, rep(t, `}`, false), "")
	assertEq(t, rep(t, `{"key": "value", "key2": 1, "key3": true}`, false),
		map[string]any{"key": "value", "key2": int64(1), "key3": true})
}

func TestBasicArrayRepair(t *testing.T) {
	assertEq(t, rep(t, `[]`, false), []any{})
	assertEq(t, rep(t, `[1, 2, 3, 4]`, false), []any{int64(1), int64(2), int64(3), int64(4)})
	assertEq(t, rep(t, `[`, false), []any{})
	assertEq(t, rep(t, `[1, 2, 3,`, false), []any{int64(1), int64(2), int64(3)})
}

func TestPythonTuples(t *testing.T) {
	assertEq(t, rep(t, `("a", "b", "c")`, false), []any{"a", "b", "c"})
	assertEq(t, rep(t, `(1)`, false), int64(1))
	assertEq(t, rep(t, `("x")`, false), "x")
	assertEq(t, rep(t, `{"coords": (1, 2), "ok": true}`, true),
		map[string]any{"coords": []any{int64(1), int64(2)}, "ok": true})
	assertEq(t, rep(t, `{"empty": ()}`, true), map[string]any{"empty": []any{}})
}

func joinStrings(parts []string) string {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	buf := make([]byte, 0, total)
	for _, p := range parts {
		buf = append(buf, p...)
	}
	return string(buf)
}
