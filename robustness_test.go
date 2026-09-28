package json_repair

import (
	"testing"
)

// hostileProbe runs a battery of nasty inputs under a recover to detect any
// panic (index-out-of-range, nil deref, etc.) that the behavior parity tests
// do not exercise. A JSON-repair library that is a fallback for
// encoding/json must never panic on hostile input.
func hostileProbe(t *testing.T) {
	t.Helper()
	inputs := []string{
		"", " ", "	\n", "{", "}", "[", "]", "{}", "[]",
		"{:}", "[:]", "{: }", "[,]", "[,,]", "{,,}",
		"{}", "{}}", "{}}}", "[[[[[[", "]]]]]]",
		"{", "{}}", "{{{", "]]]", "}{", "}{",
		`"`, `""`, `"a`, `"a"`, `"a`,
		`"\`, `"\\`, `"\\\",`, `"\u`, `"\u12`, `"\u123`, `"\u1234`,
		`"\u1234`, `"\u12345`,
		`{"a":`, `{"a": }`, `{"a":,}`, `{"a" :}`, `{a:}`, `{a b:1}`,
		`{"a":1`, `{"a":1,`, `{"a":1,}`, `{"a":1} extra`,
		`[1`, `[1,`, `[1,2`, `[1,2,`, `[1,]`, `[,1]`,
		`1 2 3`, `true false`, `null null`, `"a" "b"`,
		"{{{", "}}}", "[[[", "]]]",
		`\`, `\\`, `\x`, `\u`, `\n`,
		`0.`, `0..1`, `.5`, `1.2.3`, `1e`, `1e5`, `1e+`, `1e-`, `-`, `+`, `--`,
		`1_000`, `0x1f`, `0b101`, `0o17`,
		"{'a': 1}", `{"a": undefined}`, `{"a": NaN}`, `{"a": Infinity}`,
		`[function(){}]`, `[{a:b}]`,
		`{"a": {"b": {"c":`,
		`["a" "b" "c"]`, `{"a":"b"}`, `{"a":"b","c":"d"}`,
		`\u0000`, "\x00", "\xff", "\x7f",
		"\ufeff{}", "\ufeff[]",
		`"{"a":1}"`, `'"a":1'`,
		"```json\n{}\n```", "```\n{}\n```", "```json\n{",
		`{"a": "b", }`, `[1, 2, ]`, `{"a":1,,}`,
		" ", "   {   }   ",
		`{"key with space": 1}`, `{"":1}`, `{"a" "b": 1}`,
		`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[`,
		`{"a":{"b":{"c":{"d":{"e":{"f":{"g":`,
		`"\uD800"`, `"\uFFFF"`, `"\u{1F600}"`,
		`123456789012345678901234567890`,
		`{"a": [1,2,3,4,5,6,7,8,9,10]}`,
		`[][],[],[]`, `{}, {}, {}`,
		`[`, `{`, `]`, `}`,
	}

	combos := []Options{
		{},
		{SkipJsonLoads: true},
		{Strict: true},
		{SkipJsonLoads: true, Strict: true},
		{StreamStable: true},
		{SkipJsonLoads: true, StreamStable: true},
		{Logging: true},
		{ReturnObjects: true},
	}
	schemaCombos := []Options{
		{Schema: map[string]any{"type": "object"}},
		{Schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		{Schema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []any{"a"}}},
		{Schema: map[string]any{"type": "object"}, SchemaRepairMode: "salvage"},
	}

	panics := 0
	run := func(name, s string, o Options) {
		defer func() {
			if r := recover(); r != nil {
				panics++
				t.Errorf("PANIC in %s: input=%q opts=%+v -> %v", name, s, o, r)
			}
		}()
		v, _, err := RepairJSON(s, o)
		_ = v
		_ = err
		if !o.ReturnObjects {
			// also exercise the string path
			_, _, _ = RepairJSONString(s, o)
		}
	}
	for _, s := range inputs {
		for _, o := range combos {
			run("combo", s, o)
		}
		for _, o := range schemaCombos {
			run("schema", s, o)
		}
	}
}

func TestHostileNoPanic(t *testing.T) {
	hostileProbe(t)
}

// FuzzRepairJSON lets `go test -fuzz` discover any panic across arbitrary
// inputs and option flags. Run: go test -fuzz=FuzzRepairJSON -fuzztime=20s .
func FuzzRepairJSON(f *testing.F) {
	// seeds
	for _, s := range []string{
		"", "{", "}", "[", "]", "{:}", `"\`, `"\u12`, `{"a":`, `[1,`,
		`{"a": {"b":`, `"\uD800"`, "```json\n{", "1e", `{"a":1,,}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		combos := []Options{
			{},
			{SkipJsonLoads: true},
			{Strict: true},
			{StreamStable: true},
			{Schema: map[string]any{"type": "object"}},
			{Schema: map[string]any{"type": "array"}},
			{Schema: map[string]any{"type": "object"}, SchemaRepairMode: "salvage"},
		}
		for _, o := range combos {
			_, _, _ = RepairJSON(s, o)
			_, _, _ = RepairJSONString(s, o)
		}
	})
}
