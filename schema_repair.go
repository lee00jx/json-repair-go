package json_repair

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// SchemaRepairer mirrors Python schema_repair.SchemaRepairer. It applies schema
// rules (coercions, defaults, enum/const, required, minItems/minProperties,
// unions, $ref) to parsed values, and validates values against a JSON Schema
// via gojsonschema (DECISIONS D-005).
type SchemaRepairer struct {
	rootSchema       any
	log              *[]RepairLog // nil when logging is off
	schemaRepairMode string
	validatorCache   map[string]*gojsonschema.Schema
}

// NewSchemaRepairer mirrors Python SchemaRepairer.__init__.
func NewSchemaRepairer(schema any, log *[]RepairLog, mode *string) (*SchemaRepairer, error) {
	m, err := normalizeSchemaRepairMode(mode)
	if err != nil {
		return nil, err
	}
	// Go-only normalization (D-006): a plain map[string]any (test literals,
	// direct callers) carries no key order, so it is deep-converted once here
	// to the canonical *OrderedMap form with sorted keys (the deterministic
	// convention for orderless input). *OrderedMap and bool pass through
	// unchanged; other types are deliberately rejected LATER by ResolveSchema
	// with "Schema must be an object." (Python parity: the repairer stores the
	// raw schema and resolve_schema raises).
	if pm, ok := schema.(map[string]any); ok {
		schema = orderedMapFromPlainDeep(pm)
	}
	if d := schemaStructuralDepth(schema); d > maxSchemaNestingDepth {
		return nil, errors.New("Input schema nesting exceeds the supported schema recursion depth.")
	}
	return &SchemaRepairer{
		rootSchema:       schema,
		log:              log,
		schemaRepairMode: m,
		validatorCache:   map[string]*gojsonschema.Schema{},
	}, nil
}

// maxSchemaNestingDepth bounds the structural nesting depth of a JSON Schema we
// are willing to traverse. The Python reference relies on the interpreter's
// recursion limit to reject pathologically deep schemas (a RecursionError,
// surfaced as "Input schema nesting exceeds the supported schema recursion
// depth."). Go has no equivalent interpreter limit, so we measure the schema's
// structural depth up front and fail fast: this preserves 1:1 behaviour for the
// deep-schema tests and guards against stack overflow in gojsonschema and in the
// repairer's own recursive repair_value path.
const maxSchemaNestingDepth = 256

// schemaStructuralDepth measures how deeply nested a schema is (counting object
// nodes, arrays of subschemas, properties, allOf/oneOf/anyOf, items, etc.). A
// $ref is treated as a leaf so that circular references cannot make the walk
// unbounded (they are rejected later by resolveRef instead).
func schemaStructuralDepth(v any) int {
	switch t := v.(type) {
	case *OrderedMap:
		if t.Has("$ref") {
			return 0
		}
		max := 0
		for _, val := range t.values {
			if d := schemaStructuralDepth(val); d > max {
				max = d
			}
		}
		return 1 + max
	case map[string]any:
		if _, hasRef := t["$ref"]; hasRef {
			return 0
		}
		max := 0
		for _, val := range t {
			if d := schemaStructuralDepth(val); d > max {
				max = d
			}
		}
		return 1 + max
	case []any:
		max := 0
		for _, el := range t {
			if d := schemaStructuralDepth(el); d > max {
				max = d
			}
		}
		return 1 + max
	default:
		return 0
	}
}

// asOrderedSchema coerces a schema node to its canonical *OrderedMap form.
// Plain map[string]any nodes (only possible from direct callers / tests,
// since NewSchemaRepairer and SchemaFromInput normalize at the boundary) are
// deep-converted with sorted keys (the deterministic convention for orderless
// input, D-006). ok is false for non-object nodes.
func asOrderedSchema(v any) (*OrderedMap, bool) {
	switch t := v.(type) {
	case *OrderedMap:
		return t, true
	case map[string]any:
		return orderedMapFromPlainDeep(t), true
	}
	return nil, false
}

func (r *SchemaRepairer) recordLog(text, path string) {
	if r.log != nil {
		*r.log = append(*r.log, RepairLog{Text: text, Context: path})
	}
}

// isValid mirrors Python SchemaRepairer.is_valid.
func (r *SchemaRepairer) isValid(value, schema any) (bool, error) {
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return false, err
	}
	if resolved == true {
		return true, nil
	}
	if resolved == false {
		return false, nil
	}
	validator, err := r.getValidator(resolved)
	if err != nil {
		return false, err
	}
	res, err := validator.Validate(gojsonschema.NewGoLoader(value))
	if err != nil {
		return false, err
	}
	return res.Valid(), nil
}

// validate mirrors Python SchemaRepairer.validate: raises a ValueError-style
// error on the first violation. Error text for a root type mismatch follows the
// Python jsonschema phrasing ("X is not of type 't'"); otherwise the first
// gojsonschema error is reported.
func (r *SchemaRepairer) validate(value, schema any) error {
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return err
	}
	if resolved == true {
		return nil
	}
	if resolved == false {
		return errors.New("Schema does not allow any values.")
	}
	validator, err := r.getValidator(resolved)
	if err != nil {
		return err
	}
	res, err := validator.Validate(gojsonschema.NewGoLoader(value))
	if err != nil {
		return err
	}
	if res.Valid() {
		return nil
	}
	return errors.New(r.formatValidationError(value, resolved, res))
}

func (r *SchemaRepairer) formatValidationError(value any, resolved any, res *gojsonschema.Result) string {
	if m, ok := asOrderedSchema(resolved); ok {
		if t, ok := m.Get("type"); ok {
			if ts, isStr := t.(string); isStr && !valueMatchesJSONType(value, ts) {
				return fmt.Sprintf("%s is not of type '%s'", pythonReprScalar(value), ts)
			}
		}
	}
	for _, e := range res.Errors() {
		return normalizeValidatorErrorText(e.String())
	}
	return "Schema validation failed."
}

// normalizeValidatorErrorText bridges gojsonschema's error phrasing toward the
// Python jsonschema wording the reference tests assert on: it strips the leading
// "<instance>: " prefix (e.g. "(root): ", "$.value: ") and lowercases the first
// character, so a pattern violation reads "does not match ..." rather than
// "(root): Does not match ...". Type-mismatch and other schema-repairer errors
// are produced by our own code paths and never pass through here.
func normalizeValidatorErrorText(s string) string {
	if strings.HasPrefix(s, "(") || strings.HasPrefix(s, "$") {
		if i := strings.Index(s, ": "); i != -1 {
			s = s[i+len(": "):]
		}
	}
	if len(s) > 0 {
		s = strings.ToLower(s[:1]) + s[1:]
	}
	return s
}

// valueMatchesJSONType reports whether a parsed value satisfies a single JSON
// Schema "type" name, matching the JSON Schema type vocabulary (string, integer,
// number, boolean, null, array, object). An integer-valued float satisfies both
// "integer" and "number"; an int always satisfies "number". Used only to decide
// whether to emit the Python-style "is not of type" message before falling back
// to the gojsonschema error text (which handles pattern/enum/etc. violations).
func valueMatchesJSONType(v any, typeName string) bool {
	switch x := v.(type) {
	case nil:
		return typeName == "null"
	case bool:
		return typeName == "boolean"
	case string:
		return typeName == "string"
	case int64, uint64, int:
		return typeName == "integer" || typeName == "number"
	case float64:
		if typeName == "number" {
			return true
		}
		if typeName == "integer" {
			return x == math.Trunc(x)
		}
		return false
	case []any:
		return typeName == "array"
	case *OrderedMap, map[string]any:
		return typeName == "object"
	default:
		return false
	}
}

// getValidator mirrors Python _get_validator (with id-based cache, here keyed by
// canonical JSON of the schema, which is a stronger identity and satisfies the
// "same validator instance / single cache entry" unit test).
func (r *SchemaRepairer) getValidator(schema any) (*gojsonschema.Schema, error) {
	inlined, err := r.inlineRefsForValidation(schema)
	if err != nil {
		return nil, err
	}
	key, err := canonicalJSON(inlined)
	if err != nil {
		return nil, err
	}
	if v, ok := r.validatorCache[key]; ok {
		return v, nil
	}
	loader := gojsonschema.NewBytesLoader([]byte(key))
	v, err := gojsonschema.NewSchema(loader)
	if err != nil {
		return nil, err
	}
	r.validatorCache[key] = v
	return v, nil
}

// inlineRefsForValidation resolves every $ref in schema against the root
// document so the result compiles as a standalone gojsonschema validator. This
// mirrors Python's root_validator.evolve(schema=sub), which keeps the root
// resolver in scope so refs like "#/$defs/Item" still resolve when validating a
// union branch (or any subschema) in isolation.
func (r *SchemaRepairer) inlineRefsForValidation(schema any) (any, error) {
	return r.inlineRefs(schema, map[uintptr]bool{})
}

func (r *SchemaRepairer) inlineRefs(schema any, visited map[uintptr]bool) (any, error) {
	switch t := schema.(type) {
	case *OrderedMap:
		if ref, hasRef := t.Get("$ref"); hasRef {
			resolved, err := r.ResolveSchema(orderedMapFromPlainDeep(map[string]any{"$ref": ref}))
			if err != nil {
				return nil, err
			}
			// Cycle guard: key on the object's identity (pointer) so a
			// self-recursive schema inlines each target once instead of
			// recursing forever.
			if targetMap, ok := resolved.(*OrderedMap); ok {
				ptr := uintptr(reflect.ValueOf(targetMap).Pointer())
				if visited[ptr] {
					return resolved, nil
				}
				visited[ptr] = true
			}
			return r.inlineRefs(resolved, visited)
		}
		out := t.Clone()
		for _, k := range t.keys {
			nv, err := r.inlineRefs(t.values[k], visited)
			if err != nil {
				return nil, err
			}
			out.values[k] = nv
		}
		return out, nil
	case map[string]any:
		if ref, hasRef := t["$ref"]; hasRef {
			resolved, err := r.ResolveSchema(map[string]any{"$ref": ref})
			if err != nil {
				return nil, err
			}
			if targetMap, ok := asOrderedSchema(resolved); ok {
				ptr := uintptr(reflect.ValueOf(targetMap).Pointer())
				if visited[ptr] {
					return resolved, nil
				}
				visited[ptr] = true
			}
			return r.inlineRefs(resolved, visited)
		}
		out := make(map[string]any, len(t))
		for k, v := range t {
			nv, err := r.inlineRefs(v, visited)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, v := range t {
			nv, err := r.inlineRefs(v, visited)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	default:
		return schema, nil
	}
}

// canonicalJSON marshals a JSON value with sorted object keys (json.Marshal
// sorts map keys), giving a stable cache key.
func canonicalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// prepareSchemaForValidation mirrors Python _prepare_schema_for_validation.
func (r *SchemaRepairer) prepareSchemaForValidation(schema any) (*OrderedMap, error) {
	normalized := prepareSchemaForValidationNode(schema)
	m, ok := asOrderedSchema(normalized)
	if !ok {
		return nil, errors.New("Schema must be an object.")
	}
	return m, nil
}

// ResolveSchema mirrors Python SchemaRepairer.resolve_schema.
func (r *SchemaRepairer) ResolveSchema(schema any) (any, error) {
	if schema == nil {
		return true, nil
	}
	if _, isBool := schema.(bool); isBool {
		return schema, nil
	}
	m, ok := asOrderedSchema(schema)
	if !ok {
		return nil, schemaDefErr("Schema must be an object.")
	}
	// Python's "Schema keys must be strings" branch is unreachable here:
	// OrderedMap keys are always strings (and plain-map keys likewise).
	current := m
	seen := map[uintptr]bool{}
	for {
		ref, hasRef := current.Get("$ref")
		if !hasRef {
			return current, nil
		}
		refStr, ok := ref.(string)
		if !ok {
			return nil, schemaDefErr("$ref must be a string.")
		}
		id := schemaID(current)
		if seen[id] {
			return nil, schemaDefErr("Circular $ref detected: %s", refStr)
		}
		seen[id] = true
		resolved, err := r.resolveRef(refStr)
		if err != nil {
			return nil, err
		}
		if b, ok := resolved.(bool); ok {
			return b, nil
		}
		current, _ = resolved.(*OrderedMap)
	}
}

// schemaID returns a stable identity for an ordered schema object (pointer),
// used to detect circular $ref chains (mirrors Python's id()-based seen set).
func schemaID(m *OrderedMap) uintptr {
	if m == nil {
		return 0
	}
	return uintptr(reflect.ValueOf(m).Pointer())
}

func isString(v any) bool { _, ok := v.(string); return ok }

// IsObjectSchema mirrors Python is_object_schema.
func (r *SchemaRepairer) IsObjectSchema(schema any) bool {
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return false
	}
	m, ok := asOrderedSchema(resolved)
	if !ok {
		return false
	}
	if t, ok := m.Get("type"); ok {
		if ts, isStr := t.(string); isStr && ts == "object" {
			return true
		}
		if list, isList := t.([]any); isList {
			for _, x := range list {
				if x == "object" {
					return true
				}
			}
			return false
		}
	}
	for _, key := range []string{"properties", "patternProperties", "additionalProperties", "required"} {
		if m.Has(key) {
			return true
		}
	}
	return false
}

// IsArraySchema mirrors Python is_array_schema.
func (r *SchemaRepairer) IsArraySchema(schema any) bool {
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return false
	}
	m, ok := asOrderedSchema(resolved)
	if !ok {
		return false
	}
	if t, ok := m.Get("type"); ok {
		if ts, isStr := t.(string); isStr && ts == "array" {
			return true
		}
		if list, isList := t.([]any); isList {
			for _, x := range list {
				if x == "array" {
					return true
				}
			}
			return false
		}
	}
	return m.Has("items")
}

func (r *SchemaRepairer) allowsSchemaType(schema *OrderedMap, schemaType string) bool {
	declared, present := schema.Get("type")
	if !present {
		declared = nil
	}
	if t, ok := declared.(string); ok {
		return t == schemaType
	}
	if list, ok := declared.([]any); ok {
		for _, x := range list {
			if x == schemaType {
				return true
			}
		}
		return false
	}
	if schemaType == "object" {
		return r.IsObjectSchema(schema)
	}
	return r.IsArraySchema(schema)
}

func (r *SchemaRepairer) canSalvageListAsObject(schema *OrderedMap) bool {
	return r.allowsSchemaType(schema, "object") && !r.allowsSchemaType(schema, "array")
}

// RepairValue mirrors Python repair_value.
func (r *SchemaRepairer) RepairValue(value, schema any, path string) (any, error) {
	// Go-only normalization (D-006): values passed as plain map[string]any
	// (direct callers / tests) carry no key order; convert the whole tree to
	// the canonical *OrderedMap form with sorted keys. Parser-produced values
	// are already ordered and pass through this check untouched.
	if pm, ok := value.(map[string]any); ok {
		value = orderedMapFromPlainDeep(pm)
	}
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return nil, err
	}
	if resolved == true {
		return normalizeMissingValues(value)
	}
	if resolved == false {
		return nil, errors.New("Schema does not allow any values.")
	}
	m, _ := asOrderedSchema(resolved)
	if m == nil || m.Len() == 0 {
		return normalizeMissingValues(value)
	}

	if isMissing(value) {
		return r.fillMissing(m, path)
	}

	if allOf, ok := m.Get("allOf"); ok {
		if list, isList := allOf.([]any); isList {
			if len(list) == 0 {
				return normalizeMissingValues(value)
			}
			repaired, err := r.RepairValue(value, list[0], path)
			if err != nil {
				return nil, err
			}
			for _, sub := range list[1:] {
				repaired, err = r.RepairValue(repaired, sub, path)
				if err != nil {
					return nil, err
				}
			}
			return repaired, nil
		}
	}

	if oneOf, ok := m.Get("oneOf"); ok {
		if list, isList := oneOf.([]any); isList {
			return r.repairUnion(value, list, path)
		}
	}
	if anyOf, ok := m.Get("anyOf"); ok {
		if list, isList := anyOf.([]any); isList {
			return r.repairUnion(value, list, path)
		}
	}

	expectedType, _ := m.Get("type")
	if expectedType == nil {
		if r.IsObjectSchema(m) {
			expectedType = "object"
		} else if r.IsArraySchema(m) {
			expectedType = "array"
		}
	}

	if types, ok := expectedType.([]any); ok {
		return r.repairTypeUnion(value, types, m, path)
	}

	var repaired any
	switch {
	case expectedType == "object":
		repaired, err = r.repairObject(value, m, path)
	case expectedType == "array":
		repaired, err = r.repairArray(value, m, path)
	case isString(expectedType):
		repaired, err = r.coerceScalar(value, expectedType.(string), path)
	default:
		repaired, err = normalizeMissingValues(value)
	}
	if err != nil {
		return nil, err
	}
	return r.applyEnumConst(repaired, m, path)
}

func (r *SchemaRepairer) repairUnion(value any, schemas []any, path string) (any, error) {
	var lastErr error
	for _, sub := range schemas {
		candidate, err := r.RepairValue(deepCopyJSON(value), sub, path)
		if err == nil {
			if verr := r.validate(candidate, sub); verr == nil {
				return candidate, nil
			} else {
				lastErr = verr
				continue
			}
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, errors.New(lastErr.Error())
	}
	return nil, errors.New("No schema matched the value.")
}

func (r *SchemaRepairer) repairTypeUnion(value any, types []any, schema *OrderedMap, path string) (any, error) {
	var lastErr error
	for _, rawType := range types {
		schemaType, _ := rawType.(string)
		// Python {**schema, "type": schema_type}: existing keys keep their
		// insertion position; "type" updates in place or appends at the end.
		branch := schema.Clone()
		branch.Set("type", schemaType)
		candidate, err := r.repairByType(deepCopyJSON(value), schemaType, schema, path)
		if err != nil {
			lastErr = err
			continue
		}
		candidate, err = r.applyEnumConst(candidate, branch, path)
		if err != nil {
			lastErr = err
			continue
		}
		if verr := r.validate(candidate, branch); verr != nil {
			lastErr = verr
			continue
		}
		return candidate, nil
	}
	if lastErr != nil {
		return nil, errors.New(lastErr.Error())
	}
	return nil, errors.New("No schema type matched the value.")
}

func (r *SchemaRepairer) repairByType(value any, schemaType string, schema *OrderedMap, path string) (any, error) {
	switch schemaType {
	case "array":
		return r.repairArray(value, schema, path)
	case "object":
		return r.repairObject(value, schema, path)
	default:
		return r.coerceScalar(value, schemaType, path)
	}
}

// loadJSONStringContainer mirrors Python _load_json_string_container. wantList
// selects the expected container kind (true=list, false=dict). The strict
// decode preserves object key insertion order like Python json.loads (D-006).
func (r *SchemaRepairer) loadJSONStringContainer(value any, wantList bool, path, unwrapMsg, salvageMsg string) any {
	str, ok := value.(string)
	if !ok {
		return value
	}
	parsed, _, err := decodeOrderedStrict(str)
	if err == nil {
		if isKind(parsed, wantList) {
			r.recordLog(unwrapMsg, path)
			return parsed
		}
		return value
	}
	if r.schemaRepairMode != "salvage" {
		return value
	}
	rep, _, err := RepairJSON(str, Options{SkipJsonLoads: true})
	if err == nil && isKind(rep, wantList) {
		r.recordLog(salvageMsg, path)
		return rep
	}
	return value
}

func isKind(v any, wantList bool) bool {
	if wantList {
		_, ok := v.([]any)
		return ok
	}
	switch v.(type) {
	case *OrderedMap, map[string]any:
		return true
	}
	return false
}

// RepairArray mirrors Python _repair_array.
func (r *SchemaRepairer) repairArray(value any, schema *OrderedMap, path string) (any, error) {
	value = r.loadJSONStringContainer(value, true, path,
		"Unwrapped JSON string to array to match schema",
		"Repaired malformed JSON string to array to match schema")

	var items []any
	if list, ok := value.([]any); ok {
		items = list
	} else {
		r.recordLog("Wrapped value in array to match schema", path)
		nv, err := normalizeMissingValues(value)
		if err != nil {
			return nil, err
		}
		items = []any{nv}
	}
	salvageMode := r.schemaRepairMode == "salvage"
	cfg := arraySchemaConfig(schema)

	// Mirrors Python repair_or_drop: returns keep=false when the item was
	// dropped during salvage; returns an error otherwise (SchemaDefinitionError
	// always, non-salvage ValueError always; salvage ValueError -> dropped).
	repairOrDrop := func(rawItem, itemSchema any, itemPath string) (bool, any, error) {
		rv, err := r.RepairValue(rawItem, itemSchema, itemPath)
		if err == nil {
			return true, rv, nil
		}
		var sde *SchemaDefinitionError
		if errors.As(err, &sde) {
			return false, nil, err
		}
		if !salvageMode {
			return false, nil, err
		}
		r.recordLog("Dropped invalid array item while salvaging", itemPath)
		return false, nil, nil
	}

	if cfg.ItemsSchema != nil {
		if itemSchemas, ok := cfg.ItemsSchema.([]any); ok {
			repairedItems := []any{}
			for idx, itemSchema := range itemSchemas {
				if idx >= len(items) {
					break
				}
				itemPath := fmt.Sprintf("%s[%d]", path, idx)
				keep, rv, err := repairOrDrop(items[idx], itemSchema, itemPath)
				if err != nil {
					return nil, err
				}
				if keep {
					repairedItems = append(repairedItems, rv)
				}
			}
			if len(items) > len(itemSchemas) {
				tail := items[len(itemSchemas):]
				switch ai := cfg.AdditionalItems.(type) {
				case *OrderedMap:
					for offset, item := range tail {
						itemPath := fmt.Sprintf("%s[%d]", path, len(itemSchemas)+offset)
						keep, rv, err := repairOrDrop(item, ai, itemPath)
						if err != nil {
							return nil, err
						}
						if keep {
							repairedItems = append(repairedItems, rv)
						}
					}
				case bool:
					if ai || cfg.AdditionalItems == nil {
						for _, item := range tail {
							nv, err := normalizeMissingValues(item)
							if err != nil {
								return nil, err
							}
							repairedItems = append(repairedItems, nv)
						}
					} else {
						for offset := range tail {
							r.recordLog("Dropped extra array item not covered by schema", fmt.Sprintf("%s[%d]", path, len(itemSchemas)+offset))
						}
					}
				case nil:
					for _, item := range tail {
						nv, err := normalizeMissingValues(item)
						if err != nil {
							return nil, err
						}
						repairedItems = append(repairedItems, nv)
					}
				default:
					for offset := range tail {
						r.recordLog("Dropped extra array item not covered by schema", fmt.Sprintf("%s[%d]", path, len(itemSchemas)+offset))
					}
				}
			}
			items = repairedItems
		} else {
			repairedItems := []any{}
			for idx, item := range items {
				itemPath := fmt.Sprintf("%s[%d]", path, idx)
				keep, rv, err := repairOrDrop(item, cfg.ItemsSchema, itemPath)
				if err != nil {
					return nil, err
				}
				if keep {
					repairedItems = append(repairedItems, rv)
				}
			}
			items = repairedItems
		}
	}

	if minItems, ok := toInt64(schema.GetOrNil("minItems")); ok && int64(len(items)) < minItems {
		return nil, fmt.Errorf("Array at %s does not meet minItems.", path)
	}
	return items, nil
}

// RepairObject mirrors Python _repair_object. The returned object's key order
// mirrors Python exactly (D-006): schema properties first (schema insertion
// order, only for keys present in the value or declaring a default), then the
// remaining input keys in input (value) insertion order.
func (r *SchemaRepairer) repairObject(value any, schema *OrderedMap, path string) (any, error) {
	if r.schemaRepairMode == "salvage" && isList(value) && r.canSalvageListAsObject(schema) {
		if mapped, err := r.mapListToObject(value.([]any), schema, path); err != nil {
			return nil, err
		} else if mapped != nil {
			value = mapped
		} else if path == "$" && len(value.([]any)) == 1 {
			first := value.([]any)[0]
			if pm, isPlain := first.(map[string]any); isPlain {
				first = orderedMapFromPlainDeep(pm)
			}
			if isMap(first) {
				value = first
				r.recordLog("Unwrapped single-item root array to object while salvaging", path)
			}
		}
	}
	value = r.loadJSONStringContainer(value, false, path,
		"Unwrapped JSON string to object to match schema",
		"Repaired malformed JSON string to object to match schema")
	dict, ok := value.(*OrderedMap)
	if !ok {
		// Plain map values only reach here via direct callers with Go map
		// literals; normalize to the canonical ordered form (D-006).
		if pm, isPlain := value.(map[string]any); isPlain {
			dict = orderedMapFromPlainDeep(pm)
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("Expected object at %s, got %s.", path, jsonTypeName(value))
	}

	cfg := objectSchemaConfig(schema)

	if r.schemaRepairMode == "salvage" && len(cfg.Required) > 0 {
		// Python iterates schema_config.required, which is a set — its
		// iteration order is hash-seed dependent; sorted keys are the
		// deterministic convention for orderless input (D-006).
		withFills := dict.Clone()
		reqKeys := make([]string, 0, len(cfg.Required))
		for k := range cfg.Required {
			reqKeys = append(reqKeys, k)
		}
		for _, key := range sortedStrings(reqKeys) {
			if withFills.Has(key) {
				continue
			}
			propSchema, present := cfg.Properties.Get(key)
			if !present {
				continue
			}
			keyPath := fmt.Sprintf("%s.%s", path, key)
			filled, filledValue, err := r.fillMissingRequiredForSalvage(propSchema, keyPath)
			if err != nil {
				return nil, err
			}
			if filled {
				withFills.Set(key, filledValue)
				r.recordLog("Filled missing required property while salvaging", keyPath)
			}
		}
		dict = withFills
	}

	var missingRequired []string
	for key := range cfg.Required {
		if !dict.Has(key) {
			missingRequired = append(missingRequired, key)
		}
	}
	if len(missingRequired) > 0 {
		return nil, fmt.Errorf("Missing required properties at %s: %s", path, strings.Join(sortedStrings(missingRequired), ", "))
	}

	repaired := NewOrderedMap()
	for _, key := range cfg.PropertyKeys {
		propSchema := cfg.Properties.GetOrNil(key)
		keyPath := fmt.Sprintf("%s.%s", path, key)
		if raw, present := dict.Get(key); present {
			rv, err := r.RepairValue(raw, propSchema, keyPath)
			if err != nil {
				return nil, err
			}
			repaired.Set(key, rv)
		} else if isMap(propSchema) {
			pm, _ := propSchema.(*OrderedMap)
			if def, hasDefault := pm.Get("default"); hasDefault && !cfg.Required[key] {
				copied, err := r.copyJSONValue(def, keyPath, "default")
				if err != nil {
					return nil, err
				}
				repaired.Set(key, copied)
				r.recordLog("Inserted default value for missing property", keyPath)
			}
		}
	}

	for _, key := range dict.Keys() {
		if cfg.Properties.Has(key) {
			continue
		}
		keyPath := fmt.Sprintf("%s.%s", path, key)
		rawValue := dict.GetOrNil(key)
		matched, unsupported := matchPatternProperties(cfg.PatternProperties, key)
		for _, pattern := range unsupported {
			r.recordLog(fmt.Sprintf("Skipped unsupported patternProperties regex '%s'", pattern), keyPath)
		}
		if len(matched) > 0 {
			rv, err := r.RepairValue(rawValue, matched[0], keyPath)
			if err != nil {
				return nil, err
			}
			for _, extra := range matched[1:] {
				rv, err = r.RepairValue(rv, extra, keyPath)
				if err != nil {
					return nil, err
				}
			}
			repaired.Set(key, rv)
			continue
		}
		switch ap := cfg.AdditionalProperties.(type) {
		case *OrderedMap:
			rv, err := r.RepairValue(rawValue, ap, keyPath)
			if err != nil {
				return nil, err
			}
			repaired.Set(key, rv)
		case bool:
			if ap || cfg.AdditionalProperties == nil {
				nv, err := normalizeMissingValues(rawValue)
				if err != nil {
					return nil, err
				}
				repaired.Set(key, nv)
			} else {
				r.recordLog("Dropped extra property not covered by schema", keyPath)
			}
		case nil:
			nv, err := normalizeMissingValues(rawValue)
			if err != nil {
				return nil, err
			}
			repaired.Set(key, nv)
		default:
			r.recordLog("Dropped extra property not covered by schema", keyPath)
		}
	}

	if minProps, ok := toInt64(schema.GetOrNil("minProperties")); ok && int64(repaired.Len()) < minProps {
		return nil, fmt.Errorf("Object at %s does not meet minProperties.", path)
	}
	return repaired, nil
}

// MapListToObject mirrors Python _map_list_to_object. Keys follow the schema
// properties' insertion order (the *OrderedMap key order — 1:1 with Python's
// dict iteration, D-006).
func (r *SchemaRepairer) mapListToObject(value []any, schema *OrderedMap, path string) (any, error) {
	properties, _ := schema.Get("properties")
	pm, ok := properties.(*OrderedMap)
	if !ok || pm.Len() == 0 {
		return nil, nil
	}
	keys := pm.Keys()
	if len(value) != len(keys) {
		return nil, nil
	}
	mapped := NewOrderedMap()
	for idx, key := range keys {
		keyPath := fmt.Sprintf("%s.%s", path, key)
		rv, err := r.RepairValue(value[idx], pm.GetOrNil(key), keyPath)
		if err != nil {
			var sde *SchemaDefinitionError
			if errors.As(err, &sde) {
				return nil, err
			}
			return nil, nil
		}
		mapped.Set(key, rv)
	}
	r.recordLog("Mapped array to object by schema property order", path)
	return mapped, nil
}

func (r *SchemaRepairer) fillMissingRequiredForSalvage(schema any, path string) (bool, any, error) {
	resolved, err := r.ResolveSchema(schema)
	if err != nil {
		return false, nil, err
	}
	m, ok := asOrderedSchema(resolved)
	if !ok {
		return false, "", nil
	}
	if def, present := m.Get("default"); present {
		copied, err := r.copyJSONValue(def, path, "default")
		return true, copied, err
	}
	if c, present := m.Get("const"); present {
		copied, err := r.copyJSONValue(c, path, "const")
		return true, copied, err
	}
	if enumVals, present := m.Get("enum"); present {
		if list, isList := enumVals.([]any); isList && len(list) > 0 {
			copied, err := r.copyJSONValue(list[0], path, "enum")
			return true, copied, err
		}
	}
	expectedType, _ := m.Get("type")
	if expectedType == nil {
		if r.IsArraySchema(m) {
			expectedType = "array"
		} else if r.IsObjectSchema(m) {
			expectedType = "object"
		}
	}
	if expectedType == "array" {
		if mi, ok := toInt64(m.GetOrNil("minItems")); !ok || mi == 0 {
			return true, []any{}, nil
		}
	}
	if expectedType == "object" {
		if mp, ok := toInt64(m.GetOrNil("minProperties")); !ok || mp == 0 {
			return true, NewOrderedMap(), nil
		}
	}
	return false, "", nil
}

// FillMissing mirrors Python _fill_missing.
func (r *SchemaRepairer) fillMissing(schema *OrderedMap, path string) (any, error) {
	if c, present := schema.Get("const"); present {
		r.recordLog("Filled missing value with const", path)
		return r.copyJSONValue(c, path, "const")
	}
	if enumRaw, present := schema.Get("enum"); present {
		if enumVals, isList := enumRaw.([]any); isList {
			if len(enumVals) == 0 {
				return nil, fmt.Errorf("Enum at %s has no values.", path)
			}
			r.recordLog("Filled missing value with first enum value", path)
			return r.copyJSONValue(enumVals[0], path, "enum")
		}
	}
	if def, present := schema.Get("default"); present {
		r.recordLog("Filled missing value with default", path)
		return r.copyJSONValue(def, path, "default")
	}

	expectedType, _ := schema.Get("type")
	if types, ok := expectedType.([]any); ok {
		var lastErr error
		for _, t := range types {
			tStr, _ := t.(string)
			// Python {**schema, "type": t}: keys keep insertion order, "type"
			// updates in place or appends at the end.
			branch := schema.Clone()
			branch.Set("type", tStr)
			rv, err := r.fillMissing(branch, path)
			if err == nil {
				return rv, nil
			}
			lastErr = err
		}
		_ = lastErr
		return nil, fmt.Errorf("Cannot infer missing value at %s.", path)
	}

	if expectedType == nil {
		if r.IsObjectSchema(schema) {
			expectedType = "object"
		} else if r.IsArraySchema(schema) {
			expectedType = "array"
		}
	}

	switch expectedType {
	case "string":
		r.recordLog("Filled missing value with empty string", path)
		return "", nil
	case "integer", "number":
		r.recordLog("Filled missing value with 0", path)
		return int64(0), nil
	case "boolean":
		r.recordLog("Filled missing value with false", path)
		return false, nil
	case "array":
		if mi, ok := toInt64(schema.GetOrNil("minItems")); ok && mi > 0 {
			return nil, fmt.Errorf("Array at %s requires at least %d items.", path, mi)
		}
		r.recordLog("Filled missing value with empty array", path)
		return []any{}, nil
	case "object":
		if mp, ok := toInt64(schema.GetOrNil("minProperties")); ok && mp > 0 {
			return nil, fmt.Errorf("Object at %s requires at least %d properties.", path, mp)
		}
		r.recordLog("Filled missing value with empty object", path)
		return NewOrderedMap(), nil
	case "null":
		r.recordLog("Filled missing value with null", path)
		return nil, nil
	}
	return nil, fmt.Errorf("Cannot infer missing value at %s.", path)
}

// CoerceScalar mirrors Python _coerce_scalar.
func (r *SchemaRepairer) coerceScalar(value any, schemaType, path string) (any, error) {
	switch schemaType {
	case "string":
		if s, ok := value.(string); ok {
			return s, nil
		}
		switch n := value.(type) {
		case int64:
			r.recordLog("Coerced number to string", path)
			return strconv.FormatInt(n, 10), nil
		case uint64:
			r.recordLog("Coerced number to string", path)
			return strconv.FormatUint(n, 10), nil
		case float64:
			r.recordLog("Coerced number to string", path)
			return pythonFloatString(n), nil
		}
		return nil, fmt.Errorf("Expected string at %s.", path)

	case "integer":
		if _, isBool := value.(bool); isBool {
			return nil, fmt.Errorf("Expected integer at %s.", path)
		}
		switch n := value.(type) {
		case int64:
			return n, nil
		case uint64:
			return int64(n), nil
		case float64:
			if n == math.Trunc(n) {
				r.recordLog("Coerced number to integer", path)
				return int64(n), nil
			}
			return nil, fmt.Errorf("Expected integer at %s.", path)
		case string:
			if iv, err := strconv.ParseInt(n, 10, 64); err == nil {
				r.recordLog("Coerced string to integer", path)
				return iv, nil
			}
			num, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return nil, fmt.Errorf("Expected integer at %s.", path)
			}
			if num != math.Trunc(num) {
				return nil, fmt.Errorf("Expected integer at %s.", path)
			}
			r.recordLog("Coerced number to integer", path)
			return int64(num), nil
		}
		return nil, fmt.Errorf("Expected integer at %s.", path)

	case "number":
		if _, isBool := value.(bool); isBool {
			return nil, fmt.Errorf("Expected number at %s.", path)
		}
		switch n := value.(type) {
		case int64, uint64, float64:
			return value, nil
		case string:
			fv, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return nil, fmt.Errorf("Expected number at %s.", path)
			}
			r.recordLog("Coerced string to number", path)
			return fv, nil
		}
		return nil, fmt.Errorf("Expected number at %s.", path)

	case "boolean":
		if b, ok := value.(bool); ok {
			return b, nil
		}
		if s, ok := value.(string); ok {
			lowered := strings.ToLower(s)
			switch lowered {
			case "true", "yes", "y", "on", "1":
				r.recordLog("Coerced string to boolean", path)
				return true, nil
			case "false", "no", "n", "off", "0":
				r.recordLog("Coerced string to boolean", path)
				return false, nil
			}
		}
		switch n := value.(type) {
		case int64:
			if n == 0 || n == 1 {
				r.recordLog("Coerced number to boolean", path)
				return n == 1, nil
			}
		case uint64:
			if n == 0 || n == 1 {
				r.recordLog("Coerced number to boolean", path)
				return n == 1, nil
			}
		case float64:
			if n == 0 || n == 1 {
				r.recordLog("Coerced number to boolean", path)
				return n == 1, nil
			}
		}
		return nil, fmt.Errorf("Expected boolean at %s.", path)

	case "null":
		if value == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("Expected null at %s.", path)
	}
	return nil, schemaDefErr("Unsupported schema type %s at %s.", schemaType, path)
}

func (r *SchemaRepairer) applyEnumConst(value any, schema *OrderedMap, path string) (any, error) {
	if c, present := schema.Get("const"); present && !jsonEqual(value, c) {
		return nil, fmt.Errorf("Value at %s does not match const.", path)
	}
	if enumRaw, present := schema.Get("enum"); present {
		if enumVals, isList := enumRaw.([]any); isList {
			found := false
			for _, e := range enumVals {
				if jsonEqual(value, e) {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("Value at %s does not match enum.", path)
			}
		}
	}
	return value, nil
}

// ResolveRef mirrors Python _resolve_ref.
func (r *SchemaRepairer) resolveRef(ref string) (any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, schemaDefErr("Unsupported $ref: %s", ref)
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	current := r.rootSchema
	for _, part := range parts {
		resolved := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		m, ok := asOrderedSchema(current)
		if !ok {
			return nil, schemaDefErr("Unresolvable $ref: %s", ref)
		}
		next, present := m.Get(resolved)
		if !present {
			return nil, schemaDefErr("Unresolvable $ref: %s", ref)
		}
		current = next
	}
	switch v := current.(type) {
	case *OrderedMap:
		return v, nil
	case map[string]any:
		// The root is normalized in NewSchemaRepairer, so this is defensive
		// only; normalize for canonical downstream handling (D-006).
		return orderedMapFromPlainDeep(v), nil
	case bool:
		return v, nil
	case nil:
		return nil, schemaDefErr("Unresolvable $ref: %s", ref)
	default:
		return nil, schemaDefErr("Unresolvable $ref: %s", ref)
	}
}

// CopyJSONValue mirrors Python _copy_json_value.
func (r *SchemaRepairer) copyJSONValue(value any, path, label string) (any, error) {
	switch v := value.(type) {
	case nil, string, bool, int64, uint64, float64:
		return value, nil
	case *OrderedMap:
		out := NewOrderedMap()
		for _, key := range v.keys {
			c, err := r.copyJSONValue(v.values[key], fmt.Sprintf("%s.%s", path, key), label)
			if err != nil {
				return nil, err
			}
			out.Set(key, c)
		}
		return out, nil
	case map[string]any:
		// Plain map input is orderless; sorted keys are the deterministic
		// convention (D-006).
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		out := NewOrderedMap()
		for _, key := range sortedStrings(keys) {
			c, err := r.copyJSONValue(v[key], fmt.Sprintf("%s.%s", path, key), label)
			if err != nil {
				return nil, err
			}
			out.Set(key, c)
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			copied, err := r.copyJSONValue(item, fmt.Sprintf("%s[%d]", path, i), label)
			if err != nil {
				return nil, err
			}
			out[i] = copied
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s value at %s is not JSON compatible.", strings.Title(label), path)
}

// ---- small helpers ----

func isList(v any) bool { _, ok := v.([]any); return ok }

// isMap reports whether v is a JSON object in either Go form: the canonical
// *OrderedMap (parser/repairer output) or a plain map[string]any (direct
// caller input, orderless).
func isMap(v any) bool {
	switch v.(type) {
	case *OrderedMap, map[string]any:
		return true
	}
	return false
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		if n == math.Trunc(n) {
			return int64(n), true
		}
		return 0, false
	}
	return 0, false
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case int64, uint64, int:
		return "int"
	case float64:
		return "float"
	case []any:
		return "list"
	case *OrderedMap, map[string]any:
		return "dict"
	default:
		return reflect.TypeOf(v).Name()
	}
}

// pythonReprScalar formats a scalar the way Python repr does, for error
// messages. It is intentionally scalar-only: the full nested Python-repr
// renderer (dicts/lists with insertion order) is a known, deliberately-
// unbuilt gap (see .pmemory/modules/schema_repair.md). Non-scalars fall back
// to %v.
func pythonReprScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return pythonFloatString(x)
	case string:
		return "'" + x + "'"
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func sortedStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// jsonEqual mirrors Python `==` for JSON scalars/containers used by const/enum.
func jsonEqual(a, b any) bool {
	// Numbers: int64 and float64 compare by value (1 == 1.0).
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	// Containers: compare through plain maps so *OrderedMap objects compare
	// by content (Python dict == is key-order insensitive), not by pointer.
	return reflect.DeepEqual(toPlainValue(a), toPlainValue(b))
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
