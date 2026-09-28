package json_repair

// Translated from the authoritative Python test suite
// (../json_repair-main/tests/test_schema_repairer.py). The pydantic-model and
// module-import cases have no Go equivalent and are intentionally omitted.

import (
	"strings"
	"testing"
)

// errContains asserts err is non-nil and its message contains substr.
func errContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("error %q does not contain %q", err.Error(), substr)
	}
}

// newStd / newSalvage build repairers with a captured log slice.
func newStd(t *testing.T, schema any) (*SchemaRepairer, *[]RepairLog) {
	t.Helper()
	var log []RepairLog
	r, err := NewSchemaRepairer(schema, &log, nil)
	if err != nil {
		t.Fatalf("NewSchemaRepairer: %v", err)
	}
	return r, &log
}

func newSalvage(t *testing.T, schema any) (*SchemaRepairer, *[]RepairLog) {
	t.Helper()
	var log []RepairLog
	mode := "salvage"
	r, err := NewSchemaRepairer(schema, &log, &mode)
	if err != nil {
		t.Fatalf("NewSchemaRepairer: %v", err)
	}
	return r, &log
}

func mustVal(t *testing.T, r *SchemaRepairer, value, schema any) any {
	t.Helper()
	v, err := r.RepairValue(value, schema, "$")
	if err != nil {
		t.Fatalf("RepairValue(%v, %v): %v", value, schema, err)
	}
	return v
}

// ordSchema converts a plain map schema literal to the canonical ordered form
// (mirrors what NewSchemaRepairer / SchemaFromInput do at the boundary, D-006).
// Only needed for tests that call internal functions taking *OrderedMap
// directly.
func ordSchema(m map[string]any) *OrderedMap { return orderedMapFromPlainDeep(m) }

func strPtr(s string) *string { return &s }

func TestNormalizeMissingValuesNestedAndInvalid(t *testing.T) {
	v, err := normalizeMissingValues(MissingValue)
	if err != nil {
		t.Fatalf("normalizeMissingValues: %v", err)
	}
	assertEq(t, v, "")
	v, err = normalizeMissingValues(map[string]any{"a": MissingValue, "b": []any{MissingValue, int64(1)}})
	if err != nil {
		t.Fatalf("normalizeMissingValues: %v", err)
	}
	assertEq(t, v, map[string]any{"a": "", "b": []any{"", int64(1)}})
	// Non-string keys are unrepresentable in Go map[string]any, so the Python
	// "Object keys must be strings" branch is unreachable here (safe divergence).
	if _, err := normalizeMissingValues(func() {}); err == nil ||
		!strings.Contains(err.Error(), "JSON compatible") {
		t.Fatalf("expected 'JSON compatible' error, got %v", err)
	}
}

func TestSchemaFromInputBasic(t *testing.T) {
	v, err := SchemaFromInput(map[string]any{"type": "string"})
	if err != nil {
		t.Fatalf("SchemaFromInput: %v", err)
	}
	assertEq(t, v, map[string]any{"type": "string"})
	if v, err := SchemaFromInput(true); err != nil || v != true {
		t.Fatalf("SchemaFromInput(true) = %v, %v", v, err)
	}
	if v, err := SchemaFromInput(false); err != nil || v != false {
		t.Fatalf("SchemaFromInput(false) = %v, %v", v, err)
	}
	if _, err := SchemaFromInput(int64(1)); err == nil ||
		!strings.Contains(err.Error(), "Schema must be a JSON Schema") {
		t.Fatalf("expected 'Schema must be a JSON Schema' error, got %v", err)
	}
}

func TestSchemaRepairModeValidation(t *testing.T) {
	if m, err := normalizeSchemaRepairMode(nil); err != nil || m != "standard" {
		t.Fatalf("got %q, %v", m, err)
	}
	if m, err := normalizeSchemaRepairMode(strPtr("standard")); err != nil || m != "standard" {
		t.Fatalf("got %q, %v", m, err)
	}
	if m, err := normalizeSchemaRepairMode(strPtr("salvage")); err != nil || m != "salvage" {
		t.Fatalf("got %q, %v", m, err)
	}
	if _, err := normalizeSchemaRepairMode(strPtr("unknown")); err == nil ||
		!strings.Contains(err.Error(), "schema_repair_mode") {
		t.Fatalf("expected schema_repair_mode error, got %v", err)
	}
}

func TestSchemaRepairerValidateAndPrepare(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	if b, err := r.isValid(int64(1), true); err != nil || b != true {
		t.Fatalf("isValid(1,true) = %v, %v", b, err)
	}
	if b, err := r.isValid(int64(1), false); err != nil || b != false {
		t.Fatalf("isValid(1,false) = %v, %v", b, err)
	}
	if err := r.validate(int64(1), true); err != nil {
		t.Fatalf("validate(1,true): %v", err)
	}
	errContains(t, r.validate(int64(1), false), "Schema does not allow")

	integerSchema := map[string]any{"type": "integer"}
	if err := r.validate(int64(1), integerSchema); err != nil {
		t.Fatalf("validate(1,integer): %v", err)
	}
	v1, _ := r.getValidator(integerSchema)
	v2, _ := r.getValidator(integerSchema)
	if v1 != v2 {
		t.Fatalf("expected same validator instance for identical schema")
	}
	if _, err := r.isValid(int64(1), integerSchema); err != nil {
		t.Fatalf("isValid(1,integer): %v", err)
	}
	if _, err := r.isValid(int64(2), integerSchema); err != nil {
		t.Fatalf("isValid(2,integer): %v", err)
	}
	if len(r.validatorCache) != 1 {
		t.Fatalf("expected 1 validator cache entry, got %d", len(r.validatorCache))
	}
	errContains(t, r.validate("x", map[string]any{"type": "integer"}), "is not of type")

	schema := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}, map[string]any{"type": "string"}},
		"additionalItems": false,
	}
	if err := r.validate([]any{int64(1), "a"}, schema); err != nil {
		t.Fatalf("validate tuple: %v", err)
	}
	prepared, err := r.prepareSchemaForValidation(schema)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	pi, ok := prepared.Get("prefixItems")
	if !ok {
		t.Fatalf("expected prefixItems, got %v", prepared.Keys())
	}
	if list, isList := pi.([]any); !isList || len(list) == 0 {
		t.Fatalf("expected non-empty prefixItems, got %v", pi)
	}
	if items, _ := prepared.Get("items"); items != false {
		t.Fatalf("expected items==false, got %v", items)
	}

	schema2 := map[string]any{
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": map[string]any{"type": "string"},
	}
	prepared2, err := r.prepareSchemaForValidation(schema2)
	if err != nil {
		t.Fatalf("prepare2: %v", err)
	}
	assertEq(t, prepared2.GetOrNil("items"), map[string]any{"type": "string"})

	if _, err := r.prepareSchemaForValidation(true); err == nil ||
		!strings.Contains(err.Error(), "Schema must be an object") {
		t.Fatalf("expected 'Schema must be an object', got %v", err)
	}
}

func TestSchemaRepairerResolveSchemaAndRefs(t *testing.T) {
	root := map[string]any{
		"defs":       map[string]any{"node": map[string]any{"type": "string"}},
		"flag":       true,
		"flag_false": false,
		"bad":        int64(1),
	}
	r, _ := NewSchemaRepairer(root, nil, nil)
	assertEq(t, resolveSchemaNoErr(r, t, nil), true)
	assertEq(t, resolveSchemaNoErr(r, t, false), false)
	assertEq(t, resolveSchemaNoErr(r, t, map[string]any{"$ref": "#/defs/node"}), map[string]any{"type": "string"})
	assertEq(t, resolveSchemaNoErr(r, t, map[string]any{"$ref": "#/flag"}), true)
	assertEq(t, resolveSchemaNoErr(r, t, map[string]any{"$ref": "#/flag_false"}), false)
	errContains(t, resolveErr(t, r, "nope"), "Schema must be an object")
	// Non-string keys are unrepresentable in Go map[string]any (safe divergence).

	errContains(t, resolveRefErr(t, r, "http://example.com"), "Unsupported $ref")
	errContains(t, resolveRefErr(t, r, "#/missing"), "Unresolvable $ref")
	errContains(t, resolveRefErr(t, r, "#/bad"), "Unresolvable $ref")
}

func TestSchemaRepairerCopyJSONValueEdges(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	v, err := r.copyJSONValue(map[string]any{"a": map[string]any{"b": int64(1)}}, "$", "default")
	if err != nil {
		t.Fatalf("copyJSONValue: %v", err)
	}
	assertEq(t, v, map[string]any{"a": map[string]any{"b": int64(1)}})
	// non-string key is unrepresentable in Go (safe divergence).
	if _, err := r.copyJSONValue(struct{}{}, "$", "default"); err == nil ||
		!strings.Contains(err.Error(), "not JSON compatible") {
		t.Fatalf("expected 'not JSON compatible', got %v", err)
	}
}

func TestSchemaRepairerObjectAndArrayHelpers(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	assertEq(t, r.IsObjectSchema(map[string]any{"type": "object"}), true)
	assertEq(t, r.IsObjectSchema(map[string]any{"type": []any{"null", "object"}}), true)
	assertEq(t, r.IsObjectSchema(map[string]any{"properties": map[string]any{}}), true)
	assertEq(t, r.IsObjectSchema(map[string]any{"type": "string"}), false)

	assertEq(t, r.IsArraySchema(map[string]any{"type": "array"}), true)
	assertEq(t, r.IsArraySchema(map[string]any{"type": []any{"array", "null"}}), true)
	assertEq(t, r.IsArraySchema(map[string]any{"items": map[string]any{"type": "string"}}), true)
	assertEq(t, r.IsArraySchema(map[string]any{"type": "object"}), false)

	assertEq(t, r.IsObjectSchema(true), false)
	assertEq(t, r.IsArraySchema(false), false)
}

func TestRepairValueMissingAndUnions(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	assertEq(t, mustVal(t, r, MissingValue, map[string]any{"const": int64(1)}), int64(1))
	assertEq(t, mustVal(t, r, MissingValue, map[string]any{"enum": []any{int64(2), int64(3)}}), int64(2))
	assertEq(t, mustVal(t, r, MissingValue, map[string]any{"default": "x"}), "x")
	assertEq(t, mustVal(t, r, MissingValue, map[string]any{"type": "string"}), "")

	anySchema := map[string]any{"anyOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string"}}}
	assertEq(t, mustVal(t, r, "1", anySchema), int64(1))

	oneSchema := map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "boolean"}}}
	errContains(t, rvErr(t, r, "nope", oneSchema), "Expected boolean")

	allSchema := map[string]any{"allOf": []any{map[string]any{"type": "string"}, map[string]any{"enum": []any{"a"}}}}
	assertEq(t, mustVal(t, r, "a", allSchema), "a")
	errContains(t, rvErr(t, r, "b", allSchema), "does not match enum")

	unionSchema := map[string]any{"type": []any{"integer", "string"}}
	assertEq(t, mustVal(t, r, "2", unionSchema), int64(2))

	errContains(t, rvErr(t, r, "x", map[string]any{"oneOf": []any{}}), "No schema matched")
	errContains(t, rvErr(t, r, "x", map[string]any{"type": []any{"integer", "boolean"}}), "Expected boolean")
	errContains(t, rvErr(t, r, "x", map[string]any{"type": []any{}}), "No schema type matched")

	arrayUnion := map[string]any{"type": []any{"array", "string"}, "items": map[string]any{"type": "integer"}}
	assertEq(t, mustVal(t, r, []any{"1"}, arrayUnion), []any{int64(1)})
	objUnion := map[string]any{"type": []any{"object", "string"}, "properties": map[string]any{"a": map[string]any{"type": "integer"}}}
	assertEq(t, mustVal(t, r, map[string]any{"a": "1"}, objUnion), map[string]any{"a": int64(1)})
}

func TestTypeUnionValidatesBranchBeforeReturning(t *testing.T) {
	standard, _ := newStd(t, map[string]any{})
	salvage, _ := newSalvage(t, map[string]any{})
	schema := map[string]any{
		"type":       []any{"object", "array"},
		"properties": map[string]any{"name": map[string]any{"type": "string", "pattern": "^a+$"}},
		"required":   []any{"name"},
		"items":      map[string]any{"type": "string"},
	}
	assertEq(t, mustVal(t, standard, []any{"bbb"}, schema), []any{"bbb"})
	assertEq(t, mustVal(t, salvage, []any{"bbb"}, schema), []any{"bbb"})
}

func TestSalvageSkipsObjectMappingForMixedObjectArraySchema(t *testing.T) {
	salvage, _ := newSalvage(t, map[string]any{})
	schema := map[string]any{
		"type":       []any{"object", "array"},
		"properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}},
		"required":   []any{"x", "y"},
		"items":      map[string]any{"type": "integer"},
	}
	assertEq(t, mustVal(t, salvage, []any{int64(1), int64(2)}, schema), []any{int64(1), int64(2)})
}

func TestCanSalvageListAsObjectRequiresObjectWithoutArray(t *testing.T) {
	r, _ := newSalvage(t, map[string]any{})
	assertEq(t, r.canSalvageListAsObject(ordSchema(map[string]any{"properties": map[string]any{"a": map[string]any{"type": "integer"}}})), true)
	assertEq(t, r.canSalvageListAsObject(ordSchema(map[string]any{"items": map[string]any{"type": "integer"}})), false)
	assertEq(t, r.canSalvageListAsObject(ordSchema(map[string]any{
		"properties": map[string]any{"a": map[string]any{"type": "integer"}},
		"items":      map[string]any{"type": "integer"},
	})), false)
}

func TestRepairObjectAndArrayPaths(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	schemaObj := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "integer"},
			"b": map[string]any{"type": "string", "default": "x"},
		},
		"required":             []any{"a"},
		"patternProperties":    map[string]any{"^x": map[string]any{"type": "integer"}, "1$": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	assertEq(t, mustVal(t, r, map[string]any{"a": "1", "x1": "2", "extra": "drop"}, schemaObj),
		map[string]any{"a": int64(1), "b": "x", "x1": int64(2)})
	errContains(t, rvErr(t, r, map[string]any{}, schemaObj), "Missing required properties")

	schemaObjExtra := map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}
	assertEq(t, mustVal(t, r, map[string]any{"a": "1"}, schemaObjExtra), map[string]any{"a": int64(1)})

	schemaMinProps := map[string]any{"type": "object", "minProperties": int64(1)}
	errContains(t, rvErr(t, r, map[string]any{}, schemaMinProps), "minProperties")

	schemaBadProps := map[string]any{"type": "object", "properties": []any{}, "patternProperties": []any{}}
	assertEq(t, mustVal(t, r, map[string]any{}, schemaBadProps), map[string]any{})
	errContains(t, rvErr(t, r, []any{}, map[string]any{"type": "object"}), "Expected object")

	schemaArray := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": false,
	}
	assertEq(t, mustVal(t, r, []any{int64(1), int64(2)}, schemaArray), []any{int64(1)})
	schemaTuple := map[string]any{"type": "array", "items": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string"}}}
	assertEq(t, mustVal(t, r, []any{int64(1)}, schemaTuple), []any{int64(1)})

	schemaArrayExtra := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": map[string]any{"type": "string"},
	}
	assertEq(t, mustVal(t, r, []any{int64(1), int64(2)}, schemaArrayExtra), []any{int64(1), "2"})

	schemaArrayOpen := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": true,
	}
	assertEq(t, mustVal(t, r, []any{int64(1), "x"}, schemaArrayOpen), []any{int64(1), "x"})

	schemaArrayItems := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	assertEq(t, mustVal(t, r, []any{"1", int64(2)}, schemaArrayItems), []any{int64(1), int64(2)})
	errContains(t, rvErr(t, r, []any{"bad"}, schemaArrayItems), "Expected integer")

	schemaArrayWrap := map[string]any{"type": "array"}
	assertEq(t, mustVal(t, r, "a", schemaArrayWrap), []any{"a"})

	schemaMinItems := map[string]any{"type": "array", "minItems": int64(1)}
	errContains(t, rvErr(t, r, []any{}, schemaMinItems), "minItems")

	schemaTupleInvalid := map[string]any{"type": "array", "items": []any{map[string]any{"type": "integer"}}}
	errContains(t, rvErr(t, r, []any{"bad"}, schemaTupleInvalid), "Expected integer")

	schemaAdditionalInvalid := map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": map[string]any{"type": "integer"},
	}
	errContains(t, rvErr(t, r, []any{int64(1), "bad"}, schemaAdditionalInvalid), "Expected integer")

	salvage, _ := newSalvage(t, map[string]any{})
	mapped, merr := salvage.mapListToObject([]any{int64(1)}, ordSchema(map[string]any{"type": "object"}), "$")
	if merr != nil || mapped != nil {
		t.Fatalf("expected mapListToObject nil, got %v (err %v)", mapped, merr)
	}
	assertEq(t, mustVal(t, salvage, []any{"bad"}, schemaTupleInvalid), []any{})
	assertEq(t, mustVal(t, salvage, []any{int64(1), "bad"}, schemaAdditionalInvalid), []any{int64(1)})
	errContains(t, rvErr(t, salvage, []any{int64(1)}, map[string]any{"type": "array", "items": []any{map[string]any{"type": "bogus"}}}), "Unsupported schema type bogus")
	errContains(t, rvErr(t, salvage, []any{int64(1), int64(2)}, map[string]any{
		"type":            "array",
		"items":           []any{map[string]any{"type": "integer"}},
		"additionalItems": map[string]any{"type": "bogus"},
	}), "Unsupported schema type bogus")
	_, merr2 := salvage.mapListToObject([]any{int64(1)}, ordSchema(map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "bogus"}}}), "$")
	errContains(t, merr2, "Unsupported schema type bogus")
	// non-string property name is unrepresentable in Go (safe divergence)
}

func TestFillMissingAndCoerceScalarPaths(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	assertEq(t, mustFill(t, r, map[string]any{"type": "integer"}), int64(0))
	assertEq(t, mustFill(t, r, map[string]any{"type": "number"}), int64(0))
	assertEq(t, mustFill(t, r, map[string]any{"type": "boolean"}), false)
	assertEq(t, mustFill(t, r, map[string]any{"type": "null"}), nil)
	assertEq(t, mustFill(t, r, map[string]any{"type": "array"}), []any{})
	assertEq(t, mustFill(t, r, map[string]any{"type": "object"}), map[string]any{})
	assertEq(t, mustFill(t, r, map[string]any{"type": []any{"string", "integer"}}), "")
	assertEq(t, mustFill(t, r, map[string]any{"properties": map[string]any{"a": map[string]any{"type": "string"}}}), map[string]any{})
	assertEq(t, mustFill(t, r, map[string]any{"items": map[string]any{"type": "string"}}), []any{})
	errContains(t, fillErr(t, r, map[string]any{"type": "array", "minItems": int64(1)}), "requires at least")
	errContains(t, fillErr(t, r, map[string]any{"type": "object", "minProperties": int64(1)}), "requires at least")
	errContains(t, fillErr(t, r, map[string]any{"type": "custom"}), "Cannot infer missing value")
	errContains(t, fillErr(t, r, map[string]any{"enum": []any{}}), "has no values")
	errContains(t, fillErr(t, r, map[string]any{"type": []any{"unsupported"}}), "Cannot infer missing value")

	assertEq(t, mustCoerce(t, r, int64(1), "string"), "1")
	assertEq(t, mustCoerce(t, r, "2", "integer"), int64(2))
	assertEq(t, mustCoerce(t, r, "2.0", "integer"), int64(2))
	assertEq(t, mustCoerce(t, r, 2.0, "integer"), int64(2))
	assertEq(t, mustCoerce(t, r, "2.5", "number"), 2.5)
	assertEq(t, mustCoerce(t, r, "true", "boolean"), true)
	assertEq(t, mustCoerce(t, r, "false", "boolean"), false)
	assertEq(t, mustCoerce(t, r, "yes", "boolean"), true)
	assertEq(t, mustCoerce(t, r, "no", "boolean"), false)
	assertEq(t, mustCoerce(t, r, "y", "boolean"), true)
	assertEq(t, mustCoerce(t, r, "n", "boolean"), false)
	assertEq(t, mustCoerce(t, r, "on", "boolean"), true)
	assertEq(t, mustCoerce(t, r, "off", "boolean"), false)
	assertEq(t, mustCoerce(t, r, "1", "boolean"), true)
	assertEq(t, mustCoerce(t, r, "0", "boolean"), false)
	assertEq(t, mustCoerce(t, r, int64(1), "boolean"), true)
	assertEq(t, mustCoerce(t, r, int64(0), "boolean"), false)
	assertEq(t, mustCoerce(t, r, 1.0, "boolean"), true)
	assertEq(t, mustCoerce(t, r, 0.0, "boolean"), false)
	assertEq(t, mustCoerce(t, r, nil, "null"), nil)

	errContains(t, coerceErr(t, r, true, "string"), "Expected string")
	errContains(t, coerceErr(t, r, true, "integer"), "Expected integer")
	errContains(t, coerceErr(t, r, 2.5, "integer"), "Expected integer")
	errContains(t, coerceErr(t, r, "2.5", "integer"), "Expected integer")
	errContains(t, coerceErr(t, r, map[string]any{}, "integer"), "Expected integer")
	errContains(t, coerceErr(t, r, true, "number"), "Expected number")
	errContains(t, coerceErr(t, r, "nope", "number"), "Expected number")
	errContains(t, coerceErr(t, r, []any{}, "number"), "Expected number")
	errContains(t, coerceErr(t, r, "maybe", "boolean"), "Expected boolean")
	errContains(t, coerceErr(t, r, int64(2), "boolean"), "Expected boolean")
	errContains(t, coerceErr(t, r, "x", "null"), "Expected null")
	errContains(t, coerceErr(t, r, "x", "unsupported"), "Unsupported schema type")

	assertEq(t, mustVal(t, r, map[string]any{"a": MissingValue}, map[string]any{}), map[string]any{"a": ""})
	assertEq(t, mustVal(t, r, map[string]any{"a": MissingValue}, map[string]any{"allOf": []any{}}), map[string]any{"a": ""})
	assertEq(t, mustVal(t, r, map[string]any{"a": "1"}, map[string]any{"properties": map[string]any{"a": map[string]any{"type": "integer"}}}), map[string]any{"a": int64(1)})
	assertEq(t, mustVal(t, r, []any{"1"}, map[string]any{"items": map[string]any{"type": "integer"}}), []any{int64(1)})
	errContains(t, rvErr(t, r, int64(1), false), "Schema does not allow")
}

func TestRepairObjectSkipsUnsupportedPatternRegexWithFallbackAndLog(t *testing.T) {
	r, log := newStd(t, map[string]any{})
	schema := map[string]any{
		"type":                 "object",
		"patternProperties":    map[string]any{"^x[0-9]+$": map[string]any{"type": "integer"}},
		"additionalProperties": map[string]any{"type": "string"},
	}
	repaired := mustVal(t, r, map[string]any{"x1": int64(2)}, schema)
	assertEq(t, repaired, map[string]any{"x1": "2"})
	found := false
	for _, e := range *log {
		if strings.Contains(e.Text, "Skipped unsupported patternProperties regex '^x[0-9]+$'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported-regex log, got %v", *log)
	}
}

func TestApplyEnumConstMismatchRaises(t *testing.T) {
	r, _ := newStd(t, map[string]any{})
	errContains(t, rvErr(t, r, "b", map[string]any{"const": "a"}), "does not match const")
	errContains(t, rvErr(t, r, "b", map[string]any{"enum": []any{"a"}}), "does not match enum")
}

func TestRepairJSONValidEmptyStringReturnsEmpty(t *testing.T) {
	s, _, err := RepairJSONString(`""`, Options{})
	if err != nil {
		t.Fatalf("RepairJSONString: %v", err)
	}
	assertEq(t, s, "")
}

// ---- helpers that unwrap (value, error) pairs ----

func resolveSchemaNoErr(r *SchemaRepairer, t *testing.T, schema any) any {
	v, err := r.ResolveSchema(schema)
	if err != nil {
		t.Fatalf("ResolveSchema: %v", err)
	}
	return v
}

func rvErr(t *testing.T, r *SchemaRepairer, value, schema any) error {
	t.Helper()
	_, err := r.RepairValue(value, schema, "$")
	return err
}

func resolveErr(t *testing.T, r *SchemaRepairer, schema any) error {
	t.Helper()
	_, err := r.ResolveSchema(schema)
	return err
}

func resolveRefErr(t *testing.T, r *SchemaRepairer, ref string) error {
	t.Helper()
	_, err := r.resolveRef(ref)
	return err
}

func mustFill(t *testing.T, r *SchemaRepairer, schema map[string]any) any {
	t.Helper()
	v, err := r.fillMissing(ordSchema(schema), "$")
	if err != nil {
		t.Fatalf("fillMissing: %v", err)
	}
	return v
}

func fillErr(t *testing.T, r *SchemaRepairer, schema map[string]any) error {
	t.Helper()
	_, err := r.fillMissing(ordSchema(schema), "$")
	return err
}

func mustCoerce(t *testing.T, r *SchemaRepairer, value any, typ string) any {
	t.Helper()
	v, err := r.coerceScalar(value, typ, "$")
	if err != nil {
		t.Fatalf("coerceScalar(%v,%s): %v", value, typ, err)
	}
	return v
}

func coerceErr(t *testing.T, r *SchemaRepairer, value any, typ string) error {
	t.Helper()
	_, err := r.coerceScalar(value, typ, "$")
	return err
}
