package json_repair

// Translated from the authoritative Python test suite
// (../json_repair-main/tests/test_schema_guided_parse.py). The pydantic-model
// and module-import cases (_two_string_schema / test_schema_pydantic_*) have no
// Go equivalent and are intentionally omitted.

import (
	"strconv"
	"strings"
	"testing"
)

// ---- shared helpers for schema-guided repair_json parity ----

// repSkip mirrors repair_json(raw, schema=schema, skip_json_loads=True,
// return_objects=True[, schema_repair_mode=mode]).
func repSkip(t *testing.T, raw string, schema any, mode string) (any, error) {
	t.Helper()
	v, _, err := RepairJSON(raw, Options{Schema: schema, SkipJsonLoads: true, ReturnObjects: true, SchemaRepairMode: mode})
	return v, err
}

// repSkipLog is repSkip with logging enabled; returns the captured logs.
func repSkipLog(t *testing.T, raw string, schema any, mode string) (any, []RepairLog, error) {
	t.Helper()
	return RepairJSON(raw, Options{Schema: schema, SkipJsonLoads: true, ReturnObjects: true, SchemaRepairMode: mode, Logging: true})
}

// repFast mirrors repair_json(raw, schema=schema, return_objects=True[,
// schema_repair_mode=mode]) WITHOUT skip_json_loads (fast path available).
func repFast(t *testing.T, raw string, schema any, mode string) (any, error) {
	t.Helper()
	v, _, err := RepairJSON(raw, Options{Schema: schema, ReturnObjects: true, SchemaRepairMode: mode})
	return v, err
}

// repFastLog is repFast with logging enabled.
func repFastLog(t *testing.T, raw string, schema any, mode string) (any, []RepairLog, error) {
	t.Helper()
	return RepairJSON(raw, Options{Schema: schema, ReturnObjects: true, SchemaRepairMode: mode, Logging: true})
}

// repFastString mirrors the string-output form repair_json(..., return_objects=False).
func repFastString(t *testing.T, raw string, schema any, mode string) (string, error) {
	t.Helper()
	s, _, err := RepairJSONString(raw, Options{Schema: schema, SchemaRepairMode: mode})
	return s, err
}

// logHasText reports whether any log entry's Text equals wantText.
func logHasText(logs []RepairLog, wantText string) bool {
	for _, e := range logs {
		if e.Text == wantText {
			return true
		}
	}
	return false
}

// logHasTextAndContext reports whether any entry matches both text and context.
func logHasTextAndContext(logs []RepairLog, wantText, wantContext string) bool {
	for _, e := range logs {
		if e.Text == wantText && e.Context == wantContext {
			return true
		}
	}
	return false
}

// logTextContains reports whether any log entry's Text contains substr.
func logTextContains(logs []RepairLog, substr string) bool {
	for _, e := range logs {
		if strings.Contains(e.Text, substr) {
			return true
		}
	}
	return false
}

var twoKeyPersonSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"name": map[string]any{"type": "string"},
		"age":  map[string]any{"type": "integer"},
	},
	"required": []any{"name", "age"},
}

func TestSchemaGuidesMissingValueTypeDefaults(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text":    map[string]any{"type": "string"},
			"count":   map[string]any{"type": "integer"},
			"ratio":   map[string]any{"type": "number"},
			"flag":    map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"payload": map[string]any{"type": "object"},
			"nothing": map[string]any{"type": "null"},
		},
		"required": []any{"text", "count", "ratio", "flag", "items", "payload", "nothing"},
	}
	raw := `{ "text": , "count": , "ratio": , "flag": , "items": , "payload": , "nothing": }`
	v, err := repSkip(t, raw, schema, "")
	if err != nil {
		t.Fatalf("repSkip: %v", err)
	}
	assertEq(t, v, map[string]any{
		"text": "", "count": int64(0), "ratio": int64(0), "flag": false,
		"items": []any{}, "payload": map[string]any{}, "nothing": nil,
	})
}

func TestSchemaMissingRequiredPropertyRaises(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"required_value": map[string]any{"type": "integer", "default": int64(1)}},
		"required":   []any{"required_value"},
	}
	_, err := repSkip(t, "{}", schema, "")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageSelectsFirstMatchingTopLevelFragment(t *testing.T) {
	raw := "Here is an example: {\"foo\": 1}\n\nFinal answer:\n```json\n{\"name\": \"Alice\", \"age\": 30}\n```\n\nAlternative: {\"name\": \"Bob\", \"age\": 40}"
	v, err := repSkip(t, raw, twoKeyPersonSchema, "salvage")
	if err != nil {
		t.Fatalf("repSkip salvage: %v", err)
	}
	assertEq(t, v, map[string]any{"name": "Alice", "age": int64(30)})

	repaired, logs, err := repSkipLog(t, raw, twoKeyPersonSchema, "salvage")
	if err != nil {
		t.Fatalf("repSkipLog salvage: %v", err)
	}
	assertEq(t, repaired, map[string]any{"name": "Alice", "age": int64(30)})
	if !logHasText(logs, "Skipped top-level fragment that did not match schema while salvaging") {
		t.Fatalf("expected skip log, got %v", logs)
	}
}

func TestSchemaSalvageSkipsListFragmentBeforeMatchingObject(t *testing.T) {
	raw := "The options are [\"a\", \"b\"].\n\n{\"name\": \"Alice\", \"age\": 30}"
	v, err := repSkip(t, raw, twoKeyPersonSchema, "salvage")
	if err != nil {
		t.Fatalf("repSkip salvage: %v", err)
	}
	assertEq(t, v, map[string]any{"name": "Alice", "age": int64(30)})
}

func TestSchemaStandardStillRejectsInvalidFirstTopLevelFragment(t *testing.T) {
	_, err := repSkip(t, "{\"foo\": 1}\n{\"name\": \"Alice\", \"age\": 30}", twoKeyPersonSchema, "")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageRaisesWhenNoTopLevelFragmentMatches(t *testing.T) {
	if _, err := repSkip(t, "{\"foo\": 1}\n{\"bar\": 2}", twoKeyPersonSchema, "salvage"); err == nil {
		t.Fatalf("expected error for no matching fragment")
	} else if !strings.Contains(err.Error(), "Missing required properties") {
		t.Fatalf("got %q", err.Error())
	}
	if _, err := repSkip(t, "{\"foo\": 1} trailing prose", twoKeyPersonSchema, "salvage"); err == nil {
		t.Fatalf("expected error for trailing prose")
	} else if !strings.Contains(err.Error(), "Missing required properties") {
		t.Fatalf("got %q", err.Error())
	}
	if _, err := repSkip(t, "{\"foo\": 1} // trailing comment", twoKeyPersonSchema, "salvage"); err == nil {
		t.Fatalf("expected error for trailing comment")
	} else if !strings.Contains(err.Error(), "Missing required properties") {
		t.Fatalf("got %q", err.Error())
	}
	if _, err := repSkip(t, "", twoKeyPersonSchema, "salvage"); err == nil {
		t.Fatalf("expected error for empty input")
	} else if !strings.Contains(err.Error(), "is not of type 'object'") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestSchemaSalvageDoesNotSelectAnItemFromRealTopLevelArray(t *testing.T) {
	_, err := repSkip(t, "[{\"foo\": 1}, {\"name\": \"Alice\", \"age\": 30}]", twoKeyPersonSchema, "salvage")
	errContains(t, err, "Expected object")
}

func TestSchemaOptionalDefaultIsInserted(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"note": map[string]any{"type": "string", "default": "n/a"}},
	}
	v, err := repSkip(t, "{}", schema, "")
	if err != nil {
		t.Fatalf("repSkip: %v", err)
	}
	assertEq(t, v, map[string]any{"note": "n/a"})
}

func TestSchemaAndStrictAreMutuallyExclusive(t *testing.T) {
	_, _, err := RepairJSON("{}", Options{Schema: map[string]any{}, Strict: true, ReturnObjects: true})
	errContains(t, err, "schema and strict")
}

func TestSchemaAppliesToValidJSONWithoutSkipJsonLoads(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "integer"}},
		"required":   []any{"value"},
	}
	// Fast-path validation fails, then parser+schema fallback repairs.
	v, err := repFast(t, `{"value": "1"}`, schema, "")
	if err != nil {
		t.Fatalf("repFast: %v", err)
	}
	assertEq(t, v, map[string]any{"value": int64(1)})
	// Fast-path validation fails for a scalar, but schema-aware repair can fix it directly.
	v, err = repFast(t, `"1"`, map[string]any{"type": "integer"}, "")
	if err != nil {
		t.Fatalf("repFast scalar: %v", err)
	}
	assertEq(t, v, int64(1))
	// Fast-path validation fails for a valid scalar and parser fallback returns empty string.
	v, err = repFast(t, "true", map[string]any{"type": "string"}, "")
	if err != nil {
		t.Fatalf("repFast true: %v", err)
	}
	assertEq(t, v, "")

	_, err = repFast(t, `"bbb"`, map[string]any{"type": "string", "pattern": "^a+$"}, "")
	errContains(t, err, "does not match")
}

func TestSchemaUnionBranchKeepsRootDefsScopeDuringRepair(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Item": map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string", "pattern": "^example$"}},
				"required":   []any{"name"},
			},
		},
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Item"}},
					map[string]any{"type": "null"},
				},
			},
		},
		"required": []any{"value"},
	}

	v, err := repFast(t, `{"value": [{"name": "example"}],}`, schema, "")
	if err != nil {
		t.Fatalf("repFast: %v", err)
	}
	assertEq(t, v, map[string]any{"value": []any{map[string]any{"name": "example"}}})

	_, err = repFast(t, `{"value": [{"name": "invalid"}],}`, schema, "")
	errContains(t, err, "Expected null")
}

func TestSchemaValidFastPathKeepsLoggingEmpty(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "integer"}},
		"required":   []any{"value"},
	}
	repaired, logs, err := repFastLog(t, `{"value": 1}`, schema, "")
	if err != nil {
		t.Fatalf("repFastLog: %v", err)
	}
	assertEq(t, repaired, map[string]any{"value": int64(1)})
	if len(logs) != 0 {
		t.Fatalf("expected empty logs, got %v", logs)
	}
}

var doubleSerializedObjectSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verdict":    map[string]any{"type": "string"},
				"confidence": map[string]any{"type": "string"},
			},
			"required": []any{"verdict", "confidence"},
		},
	},
	"required": []any{"summary"},
}

var expectedDoubleSerializedObject = map[string]any{
	"summary": map[string]any{"verdict": "malicious", "confidence": "high"},
}

func TestSchemaUnwrapsDoubleSerializedObjectInAllModes(t *testing.T) {
	raw := `{"summary": "{\"verdict\": \"malicious\", \"confidence\": \"high\"}"}`
	for _, mode := range []string{"standard", "salvage"} {
		v, err := repFast(t, raw, doubleSerializedObjectSchema, mode)
		if err != nil {
			t.Fatalf("repFast(%s): %v", mode, err)
		}
		assertEq(t, v, expectedDoubleSerializedObject)
	}

	repaired, logs, err := repFastLog(t, raw, doubleSerializedObjectSchema, "standard")
	if err != nil {
		t.Fatalf("repFastLog: %v", err)
	}
	assertEq(t, repaired, expectedDoubleSerializedObject)
	if !logHasText(logs, "Unwrapped JSON string to object to match schema") {
		t.Fatalf("expected unwrap log, got %v", logs)
	}
}

func TestSchemaSalvageRepairsMalformedDoubleSerializedObjectString(t *testing.T) {
	raw := `{"summary": "{verdict: malicious, confidence: high}"}`

	_, err := repFast(t, raw, doubleSerializedObjectSchema, "standard")
	errContains(t, err, "Expected object at $.summary, got str.")

	v, err := repFast(t, raw, doubleSerializedObjectSchema, "salvage")
	if err != nil {
		t.Fatalf("repFast salvage: %v", err)
	}
	assertEq(t, v, expectedDoubleSerializedObject)

	repaired, logs, err := repFastLog(t, raw, doubleSerializedObjectSchema, "salvage")
	if err != nil {
		t.Fatalf("repFastLog salvage: %v", err)
	}
	assertEq(t, repaired, expectedDoubleSerializedObject)
	if !logHasText(logs, "Repaired malformed JSON string to object to match schema") {
		t.Fatalf("expected repair-malformed log, got %v", logs)
	}
}

var doubleSerializedArraySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []any{"items"},
}

func TestSchemaUnwrapsDoubleSerializedArrayInAllModes(t *testing.T) {
	raw := `{"items": "[\"a\", \"b\"]"}`
	expected := map[string]any{"items": []any{"a", "b"}}
	for _, mode := range []string{"standard", "salvage"} {
		v, err := repFast(t, raw, doubleSerializedArraySchema, mode)
		if err != nil {
			t.Fatalf("repFast(%s): %v", mode, err)
		}
		assertEq(t, v, expected)
	}

	repaired, logs, err := repFastLog(t, raw, doubleSerializedArraySchema, "standard")
	if err != nil {
		t.Fatalf("repFastLog: %v", err)
	}
	assertEq(t, repaired, expected)
	if !logHasText(logs, "Unwrapped JSON string to array to match schema") {
		t.Fatalf("expected unwrap-array log, got %v", logs)
	}
}

func TestSchemaSalvageRepairsMalformedDoubleSerializedArrayString(t *testing.T) {
	raw := `{"items": "[a, b]"}`
	standardExpected := map[string]any{"items": []any{"[a, b]"}}
	salvageExpected := map[string]any{"items": []any{"a", "b"}}

	v, err := repFast(t, raw, doubleSerializedArraySchema, "standard")
	if err != nil {
		t.Fatalf("repFast standard: %v", err)
	}
	assertEq(t, v, standardExpected)

	v, err = repFast(t, raw, doubleSerializedArraySchema, "salvage")
	if err != nil {
		t.Fatalf("repFast salvage: %v", err)
	}
	assertEq(t, v, salvageExpected)

	repaired, logs, err := repFastLog(t, raw, doubleSerializedArraySchema, "salvage")
	if err != nil {
		t.Fatalf("repFastLog salvage: %v", err)
	}
	assertEq(t, repaired, salvageExpected)
	if !logHasText(logs, "Repaired malformed JSON string to array to match schema") {
		t.Fatalf("expected repair-malformed-array log, got %v", logs)
	}
}

func TestSchemaObjectStringUnwrapPreservesExistingFailures(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":       "object",
				"properties": map[string]any{"verdict": map[string]any{"type": "string"}},
				"required":   []any{"verdict"},
			},
		},
		"required": []any{"summary"},
	}

	_, err := repFast(t, `{"summary": "not json"}`, schema, "")
	errContains(t, err, "Expected object at $.summary, got str.")

	_, err = repFast(t, `{"summary": "[1, 2]"}`, schema, "")
	errContains(t, err, "Expected object at $.summary, got str.")
}

func TestSchemaArrayStringUnwrapPreservesExistingFallbacks(t *testing.T) {
	v, err := repFast(t, `{"items": "not json"}`, doubleSerializedArraySchema, "")
	if err != nil {
		t.Fatalf("repFast not json: %v", err)
	}
	assertEq(t, v, map[string]any{"items": []any{"not json"}})

	v, err = repFast(t, `{"items": "{\"a\": 1}"}`, doubleSerializedArraySchema, "")
	if err != nil {
		t.Fatalf("repFast obj string: %v", err)
	}
	assertEq(t, v, map[string]any{"items": []any{`{"a": 1}`}})

	v, err = repFast(t, `{"items": "{a: 1}"}`, doubleSerializedArraySchema, "salvage")
	if err != nil {
		t.Fatalf("repFast salvage: %v", err)
	}
	assertEq(t, v, map[string]any{"items": []any{"{a: 1}"}})
}

func TestDeepAllOfSchemaRaisesValueErrorInsteadOfRecursionError(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}}
	for i := 0; i < 550; i++ {
		schema = map[string]any{"allOf": []any{schema}}
	}
	_, _, err := RepairJSON(`{"value": "ok"}`, Options{Schema: schema, ReturnObjects: true})
	errContains(t, err, "supported schema recursion depth")
}

func TestDeepPropertiesSchemaRaisesValueErrorInsteadOfRecursionError(t *testing.T) {
	schema := map[string]any{"type": "string"}
	for depth := 0; depth < 550; depth++ {
		schema = map[string]any{"type": "object", "properties": map[string]any{("level_" + strconv.Itoa(depth)): schema}}
	}
	_, _, err := RepairJSON("{}", Options{Schema: schema, ReturnObjects: true})
	errContains(t, err, "supported schema recursion depth")
}

func TestSchemaAppliesToValidJSONFastPathOutputsAndLogging(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "integer"}},
		"required":   []any{"value"},
	}
	s, err := repFastString(t, `{"value": "1"}`, schema, "")
	if err != nil {
		t.Fatalf("repFastString: %v", err)
	}
	assertEq(t, s, `{"value": 1}`)

	repaired, logs, err := repFastLog(t, `{"value": "1"}`, schema, "")
	if err != nil {
		t.Fatalf("repFastLog: %v", err)
	}
	assertEq(t, repaired, map[string]any{"value": int64(1)})
	if len(logs) == 0 {
		t.Fatalf("expected non-empty logs, got %v", logs)
	}
}

func TestSchemaPreservesPrefixedValidJSONStringContent(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"text": map[string]any{"type": "string"}, "id": map[string]any{"type": "integer"}},
					"required":             []any{"text", "id"},
					"additionalProperties": false,
				},
			},
		},
		"required": []any{"items"},
	}
	raw := "Preamble\n{\"items\": [{\"text\": \"a\\n, extra: 1\", \"id\": 8}]}"
	v, err := repFast(t, raw, schema, "")
	if err != nil {
		t.Fatalf("repFast: %v", err)
	}
	assertEq(t, v, map[string]any{"items": []any{map[string]any{"text": "a\n, extra: 1", "id": int64(8)}}})
}

func TestSchemaAppliesToValidEmptyString(t *testing.T) {
	s, err := repFastString(t, `""`, map[string]any{"type": "string"}, "")
	if err != nil {
		t.Fatalf("repFastString: %v", err)
	}
	assertEq(t, s, "")
}

func TestSchemaSkipJsonLoadsKeepsParserPathForScalars(t *testing.T) {
	v, err := repSkip(t, "True", map[string]any{"type": "string"}, "")
	if err != nil {
		t.Fatalf("repSkip True: %v", err)
	}
	assertEq(t, v, "")
	_, err = repSkip(t, `"1"`, map[string]any{"type": "integer"}, "")
	errContains(t, err, "is not of type")
}

func TestSchemaCircularRefRaisesDefinitionError(t *testing.T) {
	schema := map[string]any{
		"$ref":        "#/definitions/a",
		"definitions": map[string]any{"a": map[string]any{"$ref": "#/definitions/a"}},
	}
	_, _, err := RepairJSON("{}", Options{Schema: schema, ReturnObjects: true})
	errContains(t, err, "Circular $ref detected")
}

func TestSchemaNonStringRefRaisesDefinitionError(t *testing.T) {
	_, _, err := RepairJSON("{}", Options{Schema: map[string]any{"$ref": int64(123)}, ReturnObjects: true})
	errContains(t, err, "$ref must be a string")
}

func TestSchemaBooleanCoercionIsModeIndependent(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"flag": map[string]any{"type": "boolean"}},
		"required":   []any{"flag"},
	}
	raw := `{"flag": "yes"}`
	for _, mode := range []string{"", "standard", "salvage"} {
		v, err := repSkip(t, raw, schema, mode)
		if err != nil {
			t.Fatalf("repSkip(%q): %v", mode, err)
		}
		assertEq(t, v, map[string]any{"flag": true})
	}
}

func TestSchemaBooleanCoercionAcceptsNumberTokens(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"flag": map[string]any{"type": "boolean"}},
		"required":   []any{"flag"},
	}
	assertEq(t, mustSkip(t, `{"flag": 1}`, schema, ""), map[string]any{"flag": true})
	assertEq(t, mustSkip(t, `{"flag": 0}`, schema, ""), map[string]any{"flag": false})
	assertEq(t, mustSkip(t, `{"flag": 1.0}`, schema, ""), map[string]any{"flag": true})
	assertEq(t, mustSkip(t, `{"flag": 0.0}`, schema, ""), map[string]any{"flag": false})
}

// mustSkip is repSkip that fails the test on error.
func mustSkip(t *testing.T, raw string, schema any, mode string) any {
	t.Helper()
	v, err := repSkip(t, raw, schema, mode)
	if err != nil {
		t.Fatalf("repSkip(%q): %v", raw, err)
	}
	return v
}

func TestSchemaSalvageModeDropsInvalidArrayItems(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"id": map[string]any{"type": "integer"}, "score": map[string]any{"type": "number"}},
					"required":   []any{"id", "score"},
				},
			},
		},
		"required": []any{"items"},
	}
	raw := `{"items":[{"id":1,"score":85.6},{"id":2,"score":"N/A"}]}`

	_, err := repSkip(t, raw, schema, "")
	errContains(t, err, "Expected number")

	repaired, err := repSkip(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repSkip salvage: %v", err)
	}
	assertEq(t, repaired, map[string]any{"items": []any{map[string]any{"id": int64(1), "score": 85.6}}})

	repairedWithLogs, logs, err := repSkipLog(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repSkipLog salvage: %v", err)
	}
	assertEq(t, repairedWithLogs, map[string]any{"items": []any{map[string]any{"id": int64(1), "score": 85.6}}})
	if !logHasText(logs, "Dropped invalid array item while salvaging") {
		t.Fatalf("expected drop-item log, got %v", logs)
	}
}

func TestSchemaSalvageModeStillEnforcesMinItems(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": int64(2)}
	_, err := repSkip(t, `["1", "bad"]`, schema, "salvage")
	errContains(t, err, "minItems")
}

func TestSchemaSalvageModeDoesNotHideSchemaDefinitionErrors(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "bogus"}}
	_, err := repSkip(t, "[1]", schema, "salvage")
	errContains(t, err, "Unsupported schema type bogus")
}

func TestSchemaSalvageModeMapsListToObjectWhenUnambiguous(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"name", "tags"},
	}
	raw := `["hello", ["a", "b"]]`

	_, err := repSkip(t, raw, schema, "")
	errContains(t, err, "Expected object")

	v, err := repSkip(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repSkip salvage: %v", err)
	}
	assertEq(t, v, map[string]any{"name": "hello", "tags": []any{"a", "b"}})
}

func TestSchemaSalvageModeMapsSetLikeObjectMembersToNullValuedKeys(t *testing.T) {
	schema := map[string]any{"type": "object"}
	raw := `{"a", "b"}`

	_, err := repSkip(t, raw, schema, "standard")
	errContains(t, err, "Expected object")

	v, err := repSkip(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repSkip salvage: %v", err)
	}
	assertEq(t, v, map[string]any{"a": nil, "b": nil})

	s, _, err := RepairJSONString(raw, Options{Schema: schema, SkipJsonLoads: true, SchemaRepairMode: "salvage"})
	if err != nil {
		t.Fatalf("RepairJSONString salvage: %v", err)
	}
	assertEq(t, s, `{"a": null, "b": null}`)

	repairedWithLogs, logs, err := repSkipLog(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repSkipLog salvage: %v", err)
	}
	assertEq(t, repairedWithLogs, map[string]any{"a": nil, "b": nil})
	if !logTextContains(logs, "set-like members as null-valued object keys") {
		t.Fatalf("expected set-like log, got %v", logs)
	}
}

func TestSchemaSalvageModeSetLikeMembersDoNotOverrideMixedObjectArraySchema(t *testing.T) {
	schema := map[string]any{"type": []any{"object", "array"}, "items": map[string]any{"type": "string"}}
	v := mustSkip(t, `{"a", "b"}`, schema, "salvage")
	assertEq(t, v, []any{"a", "b"})
}

func TestSchemaSalvageModeSetLikeMembersStillFailIncompatibleObjectSchema(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "integer"}},
		"required":   []any{"count"},
	}
	_, err := repSkip(t, `{"a", "b"}`, schema, "salvage")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageModeSetLikeMembersRequireStringKeys(t *testing.T) {
	schema := map[string]any{"type": "object"}
	_, err := repSkip(t, "{1, 2}", schema, "salvage")
	errContains(t, err, "Expected object")
}

func TestSchemaSalvageModeMappingRejectsLengthMismatch(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"name", "tags"},
	}
	_, err := repSkip(t, `["hello"]`, schema, "salvage")
	errContains(t, err, "Expected object")
}

func TestSchemaSalvageModeMappingRejectsTypeMismatch(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"name", "tags"},
	}
	_, err := repSkip(t, `[["a", "b"], "hello"]`, schema, "salvage")
	errContains(t, err, "Expected object")
}

func TestSchemaSalvageModeUnionObjectArrayFallsBackToValidArrayBranch(t *testing.T) {
	schema := map[string]any{
		"type":       []any{"object", "array"},
		"properties": map[string]any{"name": map[string]any{"type": "string", "pattern": "^a+$"}},
		"required":   []any{"name"},
		"items":      map[string]any{"type": "string"},
	}
	raw := `["bbb",]`
	assertEq(t, mustFast(t, raw, schema, "standard"), []any{"bbb"})
	assertEq(t, mustFast(t, raw, schema, "salvage"), []any{"bbb"})
}

// mustFast is repFast that fails the test on error.
func mustFast(t *testing.T, raw string, schema any, mode string) any {
	t.Helper()
	v, err := repFast(t, raw, schema, mode)
	if err != nil {
		t.Fatalf("repFast(%q,%q): %v", raw, mode, err)
	}
	return v
}

func TestSchemaSalvageModeUnionObjectArrayDoesNotRemapValidArray(t *testing.T) {
	schema := map[string]any{
		"type":       []any{"object", "array"},
		"properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}},
		"required":   []any{"x", "y"},
		"items":      map[string]any{"type": "integer"},
	}
	assertEq(t, mustSkip(t, "[1,2]", schema, "salvage"), []any{int64(1), int64(2)})

	_, logs, err := repSkipLog(t, "[1,2]", schema, "salvage")
	if err != nil {
		t.Fatalf("repSkipLog: %v", err)
	}
	for _, e := range logs {
		if e.Text == "Mapped array to object by schema property order" ||
			e.Text == "Unwrapped single-item root array to object while salvaging" {
			t.Fatalf("unexpected remap log %q in %v", e.Text, logs)
		}
	}
}

func TestSchemaSalvageModeUnwrapsRootSingleItemArrayAndFillsRequiredArray(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{"const": "food_sport_card"},
			"content": map[string]any{
				"type":     "object",
				"required": []any{"food", "sports"},
				"properties": map[string]any{
					"food":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"sports": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
		},
		"required": []any{"type", "content"},
	}
	raw := "[\n    {\n        \"type\": \"food_sport_card\",\n        \"content\": {\n            \"food\": [\n                \"mantou\"\n            ]\n        }\n    }\n"

	_, err := repFast(t, raw, schema, "standard")
	errContains(t, err, "Expected object at $, got list.")

	expected := map[string]any{
		"type":    "food_sport_card",
		"content": map[string]any{"food": []any{"mantou"}, "sports": []any{}},
	}
	assertEq(t, mustFast(t, raw, schema, "salvage"), expected)

	repairedWithLogs, logs, err := repFastLog(t, raw, schema, "salvage")
	if err != nil {
		t.Fatalf("repFastLog salvage: %v", err)
	}
	assertEq(t, repairedWithLogs, expected)
	if !logHasText(logs, "Unwrapped single-item root array to object while salvaging") {
		t.Fatalf("expected unwrap log, got %v", logs)
	}
	if !logHasTextAndContext(logs, "Filled missing required property while salvaging", "$.content.sports") {
		t.Fatalf("expected fill log at $.content.sports, got %v", logs)
	}
}

func TestSchemaSalvageModeFillsRequiredWithSafeInferenceSources(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_default":      map[string]any{"default": "x"},
			"from_const":        map[string]any{"const": int64(7)},
			"from_enum":         map[string]any{"enum": []any{"first", "second"}},
			"from_array_shape":  map[string]any{"items": map[string]any{"type": "integer"}},
			"from_object_shape": map[string]any{"properties": map[string]any{"nested": map[string]any{"type": "string"}}},
		},
		"required": []any{"from_default", "from_const", "from_enum", "from_array_shape", "from_object_shape"},
	}
	assertEq(t, mustFast(t, "{}", schema, "salvage"), map[string]any{
		"from_default":      "x",
		"from_const":        int64(7),
		"from_enum":         "first",
		"from_array_shape":  []any{},
		"from_object_shape": map[string]any{},
	})
}

func TestSchemaSalvageModeMissingRequiredWithoutPropertySchemaStillRaises(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{"missing"}}
	_, err := repFast(t, "{}", schema, "salvage")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageModeMissingRequiredBooleanSchemaStillRaises(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"payload": true},
		"required":   []any{"payload"},
	}
	_, err := repFast(t, "{}", schema, "salvage")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageModeRootUnwrapRequiresSingleItem(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "integer"}},
		"required":   []any{"value"},
	}
	_, err := repFast(t, `[{"value": 1}, {"value": 2}]`, schema, "salvage")
	errContains(t, err, "Expected object at $, got list.")
}

func TestSchemaSalvageModeMissingRequiredScalarStillRaises(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
		"required":   []any{"name"},
	}
	_, err := repFast(t, "[{}]", schema, "salvage")
	errContains(t, err, "Missing required properties")
}

func TestSchemaSalvageModeRequiresSchema(t *testing.T) {
	_, _, err := RepairJSON("{}", Options{ReturnObjects: true, SchemaRepairMode: "salvage"})
	errContains(t, err, "schema_repair_mode")
}
