package json_repair

// Translated from the authoritative Python test suite
// (../json_repair-main/tests/test_schema_parser_paths.py). These exercise the
// schema-guided parser entry points (parse_object / parse_array / parse_json)
// directly rather than through repair_json.

import (
	"strings"
	"testing"
)

// ctxValue is a nullable ContextValue for the direct-parser helpers.
func ctxValue(v ContextValue) *ContextValue { return &v }

// catchSchemaPanic runs fn, converting a schemaAbort panic (raised by
// recordSchemaErr in the direct parser path, which has no ParseWithSchema
// recover wrapper) into an error. Non-schema panics are re-panicked.
func catchSchemaPanic(fn func() any) (any, error) {
	var out any
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				if sa, ok := r.(schemaAbort); ok {
					err = sa.err
					return
				}
				panic(r)
			}
		}()
		out = fn()
	}()
	return out, err
}

// dictOrEmpty mirrors Python `schema if isinstance(schema, dict) else {}` for
// the repairer's root schema (the repairer always gets a dict root).
func dictOrEmpty(schema any) any {
	if _, ok := schema.(map[string]any); ok {
		return schema
	}
	return map[string]any{}
}

func newDirectParser(raw string, strict bool, logging bool) *JSONParser {
	return newJSONParser(raw, logging, false, strict, false)
}

// parseObjectDirect mirrors the Python parse_object_direct helper.
func parseObjectDirect(raw string, schema any, strict bool, context *ContextValue) (any, error) {
	return parseObjectDirectMode(raw, schema, nil, strict, context)
}

func parseObjectDirectMode(raw string, schema any, mode *string, strict bool, context *ContextValue) (any, error) {
	return catchSchemaPanic(func() any {
		p := newDirectParser(raw, strict, false)
		r, _ := NewSchemaRepairer(dictOrEmpty(schema), nil, mode)
		p.schemaRepairer = r
		if context != nil {
			p.context.set(*context)
		}
		p.index = 1
		return p.parseObjectSchema(schema, "$")
	})
}

func parseArrayDirect(raw string, schema any) (any, error) {
	return catchSchemaPanic(func() any {
		p := newDirectParser(raw, false, false)
		r, _ := NewSchemaRepairer(dictOrEmpty(schema), nil, nil)
		p.schemaRepairer = r
		p.index = 1
		return p.parseArraySchema(schema, "$", "]")
	})
}

func parseArrayDirectMode(raw string, schema any, mode *string) (any, error) {
	return catchSchemaPanic(func() any {
		p := newDirectParser(raw, false, false)
		r, _ := NewSchemaRepairer(dictOrEmpty(schema), nil, mode)
		p.schemaRepairer = r
		p.index = 1
		return p.parseArraySchema(schema, "$", "]")
	})
}

// parseJsonDirect mirrors direct parser.parse_json(schema, "$") calls.
func parseJsonDirect(raw string, schema any, context *ContextValue) (any, error) {
	return catchSchemaPanic(func() any {
		p := newDirectParser(raw, false, false)
		r, _ := NewSchemaRepairer(dictOrEmpty(schema), nil, nil)
		p.schemaRepairer = r
		if context != nil {
			p.context.set(*context)
		}
		return p.parseJSONSchema(schema, "$", true, false)
	})
}

func TestParseObjectSchemaTrueFalseAndNonObject(t *testing.T) {
	v, err := parseObjectDirect("{}", true, false, nil)
	if err != nil {
		t.Fatalf("parseObjectDirect True: %v", err)
	}
	assertEq(t, v, map[string]any{})

	_, err = parseObjectDirect("{}", false, false, nil)
	errContains(t, err, "Schema does not allow")

	v, err = parseObjectDirect("{}", map[string]any{"type": "string"}, false, nil)
	if err != nil {
		t.Fatalf("parseObjectDirect string: %v", err)
	}
	assertEq(t, v, map[string]any{})
}

func TestParseObjectSchemaPropertyFallbacksAndStrayColon(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"properties":           []any{},
		"patternProperties":    []any{},
		"additionalProperties": true,
	}
	v, err := parseObjectDirect("{:a:1}", schema, false, nil)
	if err != nil {
		t.Fatalf("parseObjectDirect: %v", err)
	}
	assertEq(t, v, map[string]any{"a": int64(1)})
}

func TestParseObjectSchemaInvalidPropertySchemaRaises(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"a": "nope"}}
	_, err := parseObjectDirect(`{"a": 1}`, schema, false, nil)
	errContains(t, err, "Schema must be an object")
}

func TestParseObjectSchemaInvalidPatternSchemaRaises(t *testing.T) {
	schema := map[string]any{"type": "object", "patternProperties": map[string]any{"^a": "nope"}}
	_, err := parseObjectDirect(`{"a": 1}`, schema, false, nil)
	errContains(t, err, "Schema must be an object")
}

func TestParseObjectSchemaInvalidPatternExtraSchemaRaises(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"patternProperties": map[string]any{
			"^a": map[string]any{"type": "integer"},
			"a$": "nope",
		},
	}
	_, err := parseObjectDirect(`{"a": 1}`, schema, false, nil)
	errContains(t, err, "Schema must be an object")
}

func TestParseArraySchemaMissingObjectBrace(t *testing.T) {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":       "object",
			"properties": map[string]any{"a": map[string]any{"type": "integer"}},
			"required":   []any{"a"},
		},
	}
	v, err := repSkip(t, `["a": 1]`, schema, "")
	if err != nil {
		t.Fatalf("repSkip: %v", err)
	}
	assertEq(t, v, []any{map[string]any{"a": int64(1)}})
}

func TestParseObjectSchemaPatternExtraSchemas(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"patternProperties": map[string]any{
			"^x": map[string]any{"type": "integer"},
			"1$": map[string]any{"type": "integer"},
		},
	}
	v, err := repSkip(t, `{"x1": "2"}`, schema, "")
	if err != nil {
		t.Fatalf("repSkip: %v", err)
	}
	assertEq(t, v, map[string]any{"x1": int64(2)})
}

func TestParseObjectSchemaSkipsUnsupportedPatternRegexWithLog(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"patternProperties":    map[string]any{"^x[0-9]+$": map[string]any{"type": "integer"}},
		"additionalProperties": true,
	}
	var logs []RepairLog
	p := newDirectParser(`{"x1": 2}`, false, true)
	p.logger = logs
	r, _ := NewSchemaRepairer(schema, &logs, nil)
	p.schemaRepairer = r
	p.index = 1
	out, err := catchSchemaPanic(func() any { return p.parseObjectSchema(schema, "$") })
	if err != nil {
		t.Fatalf("parseObjectSchema: %v", err)
	}
	// The parser appends to its own slice field (p.log reassigns p.logger),
	// so read it back and merge, mirroring RepairJSON's trailing read-back.
	logs = append(logs, p.logger...)
	assertEq(t, out, map[string]any{"x1": int64(2)})
	if !logTextContains(logs, "Skipped unsupported patternProperties regex '^x[0-9]+$'") {
		t.Fatalf("expected unsupported-regex log, got %v", logs)
	}
}

func TestParseObjectSchemaDropPropertyAndAdditionalSchema(t *testing.T) {
	schemaDrop := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"a": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	v, err := repSkip(t, `{"a": 1, "extra": "drop"}`, schemaDrop, "")
	if err != nil {
		t.Fatalf("repSkip drop: %v", err)
	}
	assertEq(t, v, map[string]any{"a": int64(1)})

	schemaExtra := map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "integer"},
	}
	v, err = repSkip(t, `{"a": "2"}`, schemaExtra, "")
	if err != nil {
		t.Fatalf("repSkip extra: %v", err)
	}
	assertEq(t, v, map[string]any{"a": int64(2)})
}

func TestParseObjectSchemaClosingArrayBracketAndExtraBrace(t *testing.T) {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":       "object",
			"properties": map[string]any{"a": map[string]any{"type": "integer"}},
			"required":   []any{"a"},
		},
	}
	v, err := repSkip(t, `[{"a": 1]`, schema, "")
	if err != nil {
		t.Fatalf("repSkip: %v", err)
	}
	assertEq(t, v, []any{map[string]any{"a": int64(1)}})

	schemaObj := map[string]any{"type": "object", "additionalProperties": true}
	var logs []RepairLog
	p := newDirectParser(`{"a": 1}}`, false, false)
	p.logger = logs
	r, _ := NewSchemaRepairer(schemaObj, nil, nil)
	p.schemaRepairer = r
	p.context.set(ContextArray)
	p.index = 1
	out, err := catchSchemaPanic(func() any { return p.parseObjectSchema(schemaObj, "$") })
	if err != nil {
		t.Fatalf("parseObjectSchema: %v", err)
	}
	assertEq(t, out, map[string]any{"a": int64(1)})
}

func TestParseObjectSchemaEmptyObjectFallsBackToArray(t *testing.T) {
	schema := map[string]any{"type": "object", "additionalProperties": true}
	v, err := parseObjectDirect("{,,}", schema, false, nil)
	if err != nil {
		t.Fatalf("parseObjectDirect: %v", err)
	}
	assertEq(t, v, []any{})
}

func TestParseObjectSchemaSalvageSetLiteralKeepsNonStringMembersAsArray(t *testing.T) {
	schema := map[string]any{"type": "object"}
	mode := "salvage"
	v, err := parseObjectDirectMode("{1, 2}", schema, &mode, false, nil)
	if err != nil {
		t.Fatalf("parseObjectDirectMode: %v", err)
	}
	assertEq(t, v, []any{int64(1), int64(2)})
}

func TestParseArraySchemaTrueFalseAndNonArray(t *testing.T) {
	v, err := parseArrayDirect("[1]", true)
	if err != nil {
		t.Fatalf("parseArrayDirect True: %v", err)
	}
	assertEq(t, v, []any{int64(1)})

	_, err = parseArrayDirect("[1]", false)
	errContains(t, err, "Schema does not allow")

	v, err = parseArrayDirect("[1]", map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("parseArrayDirect object: %v", err)
	}
	assertEq(t, v, []any{int64(1)})
}

func TestParseArraySchemaItemsAndAdditionalItems(t *testing.T) {
	schemaDrop := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": false,
	}
	v, err := parseArrayDirect("[1, 2]", schemaDrop)
	if err != nil {
		t.Fatalf("parseArrayDirect drop: %v", err)
	}
	assertEq(t, v, []any{int64(1)})

	schemaExtra := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": map[string]any{"type": "integer"},
	}
	v, err = parseArrayDirect(`[1, "2"]`, schemaExtra)
	if err != nil {
		t.Fatalf("parseArrayDirect extra: %v", err)
	}
	assertEq(t, v, []any{int64(1), int64(2)})

	schemaOpen := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": true,
	}
	v, err = parseArrayDirect(`[1, "x"]`, schemaOpen)
	if err != nil {
		t.Fatalf("parseArrayDirect open: %v", err)
	}
	assertEq(t, v, []any{int64(1), "x"})

	schemaItems := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	v, err = parseArrayDirect(`["1"]`, schemaItems)
	if err != nil {
		t.Fatalf("parseArrayDirect items: %v", err)
	}
	assertEq(t, v, []any{int64(1)})

	schemaAny := map[string]any{"type": "array"}
	v, err = parseArrayDirect("[true]", schemaAny)
	if err != nil {
		t.Fatalf("parseArrayDirect any: %v", err)
	}
	assertEq(t, v, []any{true})
}

func TestParseArraySchemaInvalidItemsSchemaRaises(t *testing.T) {
	schema := map[string]any{"type": "array", "items": []any{"nope"}}
	_, err := parseArrayDirect("[1]", schema)
	errContains(t, err, "Schema must be an object")
}

func TestParseJsonWithSchemaBranches(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	arrCtx := ctxValue(ContextArray)

	v, err := parseJsonDirect("[1]", schema, nil)
	if err != nil {
		t.Fatalf("parseJsonDirect [1]: %v", err)
	}
	assertEq(t, v, []any{int64(1)})

	v, err = parseJsonDirect(`"1"`, map[string]any{"type": "integer"}, arrCtx)
	if err != nil {
		t.Fatalf("parseJsonDirect \"1\": %v", err)
	}
	assertEq(t, v, int64(1))

	v, err = parseJsonDirect("1", map[string]any{"type": "integer"}, arrCtx)
	if err != nil {
		t.Fatalf("parseJsonDirect 1: %v", err)
	}
	assertEq(t, v, int64(1))

	v, err = parseJsonDirect("# comment", map[string]any{"type": "string"}, nil)
	if err != nil {
		t.Fatalf("parseJsonDirect comment: %v", err)
	}
	assertEq(t, v, "")

	v, err = parseJsonDirect("", map[string]any{"type": "string"}, nil)
	if err != nil {
		t.Fatalf("parseJsonDirect empty: %v", err)
	}
	assertEq(t, v, "")

	v, err = parseJsonDirect(`"x"`, true, arrCtx)
	if err != nil {
		t.Fatalf("parseJsonDirect true: %v", err)
	}
	assertEq(t, v, "x")

	_, err = parseJsonDirect(`"x"`, false, arrCtx)
	errContains(t, err, "Schema does not allow")

	v, err = parseJsonDirect(`@{"a": 1}`, map[string]any{"type": "object"}, nil)
	if err != nil {
		t.Fatalf("parseJsonDirect @object: %v", err)
	}
	assertEq(t, v, map[string]any{"a": int64(1)})
}

func TestSchemaRefToTrueShortCircuits(t *testing.T) {
	refSchema := map[string]any{"$ref": "#/flag"}
	_ = refSchema

	v, err := catchSchemaPanic(func() any {
		p := newDirectParser("[1]", false, false)
		r, _ := NewSchemaRepairer(map[string]any{"flag": true}, nil, nil)
		p.schemaRepairer = r
		p.index = 1
		return p.parseArraySchema(map[string]any{"$ref": "#/flag"}, "$", "]")
	})
	if err != nil {
		t.Fatalf("parseArraySchema ref-true: %v", err)
	}
	assertEq(t, v, []any{int64(1)})

	v, err = catchSchemaPanic(func() any {
		p := newDirectParser(`{"a": 1}`, false, false)
		r, _ := NewSchemaRepairer(map[string]any{"flag": true}, nil, nil)
		p.schemaRepairer = r
		p.index = 1
		return p.parseObjectSchema(map[string]any{"$ref": "#/flag"}, "$")
	})
	if err != nil {
		t.Fatalf("parseObjectSchema ref-true: %v", err)
	}
	assertEq(t, v, map[string]any{"a": int64(1)})

	v, err = catchSchemaPanic(func() any {
		p := newDirectParser("1", false, false)
		r, _ := NewSchemaRepairer(map[string]any{"flag": true}, nil, nil)
		p.schemaRepairer = r
		p.context.set(ContextArray)
		return p.parseJSONSchema(map[string]any{"$ref": "#/flag"}, "$", true, false)
	})
	if err != nil {
		t.Fatalf("parseJSONSchema ref-true: %v", err)
	}
	assertEq(t, v, int64(1))
}

func TestParseArraySalvageModeParsesEvenWhenItemSchemaWouldFail(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	mode := "salvage"
	v, err := parseArrayDirectMode(`["bad", "2"]`, schema, &mode)
	if err != nil {
		t.Fatalf("parseArrayDirectMode salvage: %v", err)
	}
	assertEq(t, v, []any{"bad", "2"})
}

func TestParseArraySalvageModeKeepsObjectHeuristicValues(t *testing.T) {
	schema := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "integer"}}},
	}
	mode := "salvage"
	v, err := parseArrayDirectMode(`["a": 1]`, schema, &mode)
	if err != nil {
		t.Fatalf("parseArrayDirectMode salvage: %v", err)
	}
	assertEq(t, v, []any{map[string]any{"a": int64(1)}})
}

// ensure strings import is used (guards against accidental removal).
var _ = strings.Contains
