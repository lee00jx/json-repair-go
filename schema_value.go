package json_repair

import (
	"fmt"
)

// missingValueType is the sentinel type for a value that was entirely absent
// from the input (mirrors Python MISSING_VALUE / MissingValueType). The parser
// hands a *missingValueType to the schema repairer where a required value was
// not present, so the repairer can fill defaults/const/enum or infer by type.
type missingValueType struct{}

// MissingValue is the exported sentinel (Python MISSING_VALUE).
var MissingValue = &missingValueType{}

// isMissing reports whether v is the missing sentinel.
func isMissing(v any) bool { _, ok := v.(*missingValueType); return ok }

// SchemaDefinitionError mirrors Python's SchemaDefinitionError (a ValueError
// subclass) for invalid/unsupported schema metadata.
type SchemaDefinitionError struct{ msg string }

func (e *SchemaDefinitionError) Error() string { return e.msg }

func schemaDefErr(format string, a ...any) error {
	return &SchemaDefinitionError{msg: fmt.Sprintf(format, a...)}
}

// SchemaFromInput mirrors Python schema_from_input (the JSON Schema subset).
// A pydantic-style model has no Go equivalent, so non-dict/non-bool inputs are
// rejected with the "Schema must be a JSON Schema" error. The accepted dict
// forms are *OrderedMap (the canonical production form) and plain
// map[string]any (converted to an ordered tree with sorted keys — the
// deterministic convention for orderless input, DECISIONS D-006).
func SchemaFromInput(schema any) (any, error) {
	switch schema.(type) {
	case *OrderedMap:
		return schema, nil
	case map[string]any:
		return orderedMapFromPlainDeep(schema.(map[string]any)), nil
	case bool:
		return schema, nil
	}
	return nil, fmt.Errorf("Schema must be a JSON Schema dict or boolean schema.")
}

// normalizeSchemaRepairMode mirrors Python normalize_schema_repair_mode.
func normalizeSchemaRepairMode(mode *string) (string, error) {
	if mode == nil {
		return "standard", nil
	}
	switch *mode {
	case "standard":
		return "standard", nil
	case "salvage":
		return "salvage", nil
	default:
		return "", fmt.Errorf("schema_repair_mode must be one of: standard, salvage.")
	}
}

// normalizeMissingValues mirrors Python normalize_missing_values: replaces the
// missing sentinel with "" and validates JSON compatibility.
func normalizeMissingValues(value any) (any, error) {
	if isMissing(value) {
		return "", nil
	}
	switch v := value.(type) {
	case *OrderedMap:
		out := NewOrderedMap()
		for _, k := range v.keys {
			nv, err := normalizeMissingValues(v.values[k])
			if err != nil {
				return nil, err
			}
			out.Set(k, nv)
		}
		return out, nil
	case map[string]any:
		// Plain map input is orderless; sorted keys are the deterministic
		// convention (D-006). Output is the canonical ordered form.
		out := NewOrderedMap()
		for _, k := range sortedStrings(sortedKeys(v)) {
			nv, err := normalizeMissingValues(v[k])
			if err != nil {
				return nil, err
			}
			out.Set(k, nv)
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			nv, err := normalizeMissingValues(item)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	case nil, string, int64, uint64, int, float64, bool:
		return value, nil
	default:
		return nil, fmt.Errorf("Value is not JSON compatible.")
	}
}

// jsonIsCompatible reports whether v is a valid JSON value (used for
// non-string-key / compatibility checks).
func jsonIsCompatible(v any) bool {
	switch v.(type) {
	case nil, string, int64, uint64, int, float64, bool, *OrderedMap, map[string]any, []any:
		return true
	default:
		return false
	}
}

// deepCopyJSON deep-copies a JSON value (mirrors Python copy.deepcopy in the
// union repair branches, which must not mutate shared input). Plain map input
// is deep-copied into the canonical ordered form (sorted keys, D-006).
func deepCopyJSON(v any) any {
	switch x := v.(type) {
	case *OrderedMap:
		out := x.Clone()
		for _, k := range x.keys {
			out.values[k] = deepCopyJSON(x.values[k])
		}
		return out
	case map[string]any:
		out := NewOrderedMap()
		for _, k := range sortedStrings(sortedKeys(x)) {
			out.values[k] = deepCopyJSON(x[k])
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = deepCopyJSON(item)
		}
		return out
	default:
		return v
	}
}

// sortedKeys returns the keys of a plain map as a slice (unsorted).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// prepareSchemaForValidationNode mirrors Python _prepare_schema_for_validation_node:
// converts legacy array-form "items" into "prefixItems" for a modern validator.
// NOTE: gojsonschema consumes the raw draft-04 array-form items directly, so the
// SchemaRepairer feeds the RAW schema to gojsonschema; this transform is kept for
// behavioural parity (it is unit-tested) and for documentation.
func prepareSchemaForValidationNode(node any) any {
	switch t := node.(type) {
	case *OrderedMap:
		normalized := t.Clone()
		for _, k := range t.keys {
			normalized.values[k] = prepareSchemaForValidationNode(t.values[k])
		}
		if items, ok := normalized.values["items"].([]any); ok {
			normalized.Delete("items")
			normalized.Set("prefixItems", items)
			if ai, present := normalized.Get("additionalItems"); present {
				normalized.Delete("additionalItems")
				switch x := ai.(type) {
				case bool:
					if !x {
						normalized.Set("items", false)
					}
				case *OrderedMap:
					normalized.Set("items", x)
				}
			}
		}
		return normalized
	case map[string]any:
		// Orderless input: convert to the canonical ordered form first
		// (sorted keys, D-006) so the items→prefixItems transform applies.
		return prepareSchemaForValidationNode(orderedMapFromPlainDeep(t))
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = prepareSchemaForValidationNode(item)
		}
		return out
	default:
		return node
	}
}
