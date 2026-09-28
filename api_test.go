package json_repair

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestRepairJSONString mirrors repair_json(..., return_objects=False) output.
func TestRepairJSONString(t *testing.T) {
	cases := []struct {
		in   string
		opts Options
		want string
	}{
		{in: "{key:value}", want: `{"key": "value"}`},
		{in: `Hello {"k": 1}`, opts: Options{EnsureASCII: false}, want: `{"k": 1}`},
		{in: `{"a":"中文"}`, opts: Options{EnsureASCII: true}, want: "{\"a\": \"\\u4e2d\\u6587\"}"},
		{in: `{"a":"中文"}`, opts: Options{EnsureASCII: false}, want: "{\"a\": \"中文\"}"},
		{in: "", want: ""},
		{in: "abc", want: ""},
	}
	for _, c := range cases {
		got, _, err := RepairJSONString(c.in, c.opts)
		if err != nil {
			t.Fatalf("RepairJSONString(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("RepairJSONString(%q):\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

// TestFromFileLogging locks in the exact repair-log sequence for the
// {key:value} case (parity with test_repair_json_from_file.py).
func TestFromFileLogging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp.json")
	if err := os.WriteFile(path, []byte("{key:value}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, logs, err := FromFile(path, Options{Logging: true})
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	want := map[string]any{"key": "value"}
	if !reflect.DeepEqual(toPlainValue(got), want) {
		t.Fatalf("result: got %#v want %#v", got, want)
	}
	wantTexts := []string{
		"While parsing a string, we found a literal instead of a quote",
		"While parsing a string missing the left delimiter in object key context, we found a :, stopping here",
		"While parsing a string, we missed the closing quote, ignoring",
		"While parsing a string, we found a literal instead of a quote",
		"While parsing a string missing the left delimiter in object value context, we found a , or } and we couldn't determine that a right delimiter was present. Stopping here",
		"While parsing a string, we missed the closing quote, ignoring",
	}
	if len(logs) != len(wantTexts) {
		t.Fatalf("log count: got %d want %d", len(logs), len(wantTexts))
	}
	for i, w := range wantTexts {
		if logs[i].Text != w {
			t.Errorf("log[%d].Text:\n got  %q\n want %q", i, logs[i].Text, w)
		}
	}
}

// TestDecodeJSON locks in the int/float type mapping (D-002) for the
// strict single-value decode used by the CLI schema-file loader.
func TestDecodeJSON(t *testing.T) {
	v, err := DecodeJSON(`{"a": 1, "b": 1.5, "c": "x", "d": [2, -3], "e": null, "f": true}`)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	want := map[string]any{
		"a": int64(1), "b": 1.5, "c": "x",
		"d": []any{int64(2), int64(-3)}, "e": nil, "f": true,
	}
	if !reflect.DeepEqual(toPlainValue(v), want) {
		t.Fatalf("DecodeJSON: got %#v want %#v", v, want)
	}

	if _, err := DecodeJSON(`{"a": 1} extra`); err == nil {
		t.Fatal("expected error for trailing data")
	}
	if _, err := DecodeJSON(`{`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
