package json_repair

import (
	"fmt"
	"strings"
	"unicode"
)

// parseObject is the non-schema entry point.
func (p *JSONParser) parseObject() any {
	return p.parseObjectSchema(nil, "$")
}

// parseObjectSchema ports the Python parse_object parser with schema guidance:
// the member loop with missing-quote / missing-colon recovery, array-continuation
// merge, the empty-object classifier (set-like `{a, b}` and backslash-escaped-key
// inputs reparsed as array/object, and the salvage "schema_set_object" case),
// duplicate-key splitting, and the trailing ", key: value" merge. When a schema
// repairer is active it guides per-key parsing (MISSING_VALUE fills, property /
// patternProperties / additionalProperties schemas) and finalizes required and
// default properties.
func (p *JSONParser) parseObjectSchema(schema any, path string) any {
	obj := NewOrderedMap()
	start := p.index
	parsingObjectValue := p.context.currentIs(ContextObjectValue)
	lastKey := ""
	schemaReparer, schema, schemaConfig, rerr := resolveParserObjectSchema(p.schemaRepairer, schema)
	if rerr != nil {
		p.recordSchemaErr(rerr)
	}

	for {
		if p.charAtOr(0, '}') == '}' {
			break
		}
		p.skipWhitespaces()
		if p.charAtOr(0, 0) == ':' {
			p.log("While parsing an object we found a : before a key, ignoring")
			p.index++
		}

		key, rollbackIndex := p.parseObjectKey(obj, lastKey)

		if key == "" && p.strict && obj.Len() == 0 && p.charAtOr(0, 0) == ':' {
			p.fail("Empty key found in strict mode")
		}

		if p.context.in(ContextArray) {
			if obj.Has(key) {
				if p.strict {
					p.log("Duplicate key found in strict mode while parsing object, raising an error")
					p.fail(fmt.Sprintf("Duplicate key found in strict mode while parsing object: %q", key))
				} else if !parsingObjectValue {
					if p.shouldSplitDuplicateObject(rollbackIndex) {
						p.log("While parsing an object we found a duplicate key, closing the object here and rolling back the index")
						p.splitObjectOnDuplicateKey(rollbackIndex)
						break
					}
					p.log("While parsing an object we found a duplicate key with a normal comma separator, keeping duplicate-key overwrite behavior")
				}
			}
		}

		p.skipWhitespaces()
		if p.charAtOr(0, '}') == '}' {
			continue
		}
		p.skipWhitespaces()
		if p.charAtOr(0, 0) != ':' {
			if p.strict {
				p.log("Missing ':' after key in strict mode while parsing object, raising an error")
				p.fail("Missing ':' after key in strict mode while parsing object")
			} else {
				p.log("While parsing an object we missed a : after a key")
			}
		}
		p.index++ // consume ':' (or the position where it was expected)

		propSchema, extraSchemas, dropProperty := p.resolveObjectPropertySchema(schemaReparer, schemaConfig, key)
		keyPath := fmt.Sprintf("%s.%s", path, key)
		value := p.parseObjectValueSchema(schemaReparer, propSchema, keyPath)

		if schemaReparer != nil {
			for _, extra := range extraSchemas {
				rv, err := schemaReparer.RepairValue(value, extra, keyPath)
				if err != nil {
					p.recordSchemaErr(err)
				} else {
					value = rv
				}
			}
		}

		if schemaReparer == nil && value == "" && p.strict {
			if prev, _ := p.charAt(-1); !isStringDelimiter(prev) {
				p.log("Parsed value is empty in strict mode while parsing object, raising an error")
				p.fail("Parsed value is empty in strict mode while parsing object")
			}
		}

		if schemaReparer == nil || !dropProperty {
			if !obj.Has(key) {
				lastKey = key
			}
			obj.Set(key, value)
		} else {
			schemaReparer.recordLog("Dropped extra property not covered by schema", keyPath)
		}

		if c, _ := p.charAt(0); c == ',' || c == '\'' || c == '"' {
			p.index++
		}
		if p.charAtOr(0, 0) == ']' && p.context.in(ContextArray) {
			p.log("While parsing an object we found a closing array bracket, closing the object here and rolling back the index")
			p.index--
			break
		}
		p.skipWhitespaces()
	}

	p.index++ // consume '}'

	if repaired, val := p.repairEmptyObjectResult(obj, start, schema, schemaReparer); repaired {
		return val
	}
	return p.completeObjectParse(obj, schema, path, schemaReparer, schemaConfig)
}

// resolveObjectPropertySchema mirrors Python _resolve_object_property_schema.
func (p *JSONParser) resolveObjectPropertySchema(schemaReparer *SchemaRepairer, cfg *ObjectSchemaConfig, key string) (any, []any, bool) {
	if schemaReparer == nil || cfg == nil {
		return nil, nil, false
	}
	if ps, ok := cfg.Properties.Get(key); ok {
		if ps != nil && !isSchemaType(ps) {
			p.recordSchemaErr(schemaDefErr("Schema must be an object."))
			return nil, nil, false
		}
		return ps, nil, false
	}
	matched, unsupported := matchPatternProperties(cfg.PatternProperties, key)
	for _, pattern := range unsupported {
		p.log(fmt.Sprintf("Skipped unsupported patternProperties regex '%s' while parsing object key '%s'", pattern, key))
	}
	if len(matched) > 0 {
		primary := matched[0]
		if primary != nil && !isSchemaType(primary) {
			p.recordSchemaErr(schemaDefErr("Schema must be an object."))
			return nil, nil, false
		}
		var extraSchemas []any
		for _, extra := range matched[1:] {
			if extra != nil && !isSchemaType(extra) {
				p.recordSchemaErr(schemaDefErr("Schema must be an object."))
				return nil, nil, false
			}
			extraSchemas = append(extraSchemas, extra)
		}
		return primary, extraSchemas, false
	}
	if b, ok := cfg.AdditionalProperties.(bool); ok && !b {
		return nil, nil, true
	}
	if m, ok := cfg.AdditionalProperties.(*OrderedMap); ok {
		return m, nil, false
	}
	return true, nil, false
}

// parseObjectValueSchema mirrors Python _parse_object_value.
func (p *JSONParser) parseObjectValueSchema(schemaReparer *SchemaRepairer, propSchema any, keyPath string) any {
	p.context.set(ContextObjectValue)
	defer p.context.reset()
	p.skipWhitespaces()
	if c, ok := p.charAt(0); ok && (c == ',' || c == '}') {
		p.log(fmt.Sprintf("While parsing an object value we found a stray %c, ignoring it", c))
		if schemaReparer != nil {
			v, err := schemaReparer.RepairValue(MissingValue, propSchema, keyPath)
			if err != nil {
				p.recordSchemaErr(err)
				return ""
			}
			return v
		}
		return ""
	}
	if schemaReparer != nil {
		return p.parseJSONSchema(propSchema, keyPath, true, false)
	}
	return p.parseJSON()
}

// finalizeObject mirrors Python _finalize_object: raises (records) for missing
// required properties in standard mode and inserts defaults for missing
// non-required properties that declare a default, in schema property order
// (Python iterates the properties dict in insertion order; D-006).
func (p *JSONParser) finalizeObject(obj *OrderedMap, schemaReparer *SchemaRepairer, cfg *ObjectSchemaConfig, path string) *OrderedMap {
	if schemaReparer == nil || cfg == nil {
		return obj
	}
	var missingRequired []string
	for k := range cfg.Required {
		if !obj.Has(k) {
			missingRequired = append(missingRequired, k)
		}
	}
	if len(missingRequired) > 0 && schemaReparer.schemaRepairMode != "salvage" {
		p.recordSchemaErr(fmt.Errorf("Missing required properties at %s: %s", path, strings.Join(sortedStrings(missingRequired), ", ")))
		return obj
	}
	for _, key := range cfg.PropertyKeys {
		if obj.Has(key) {
			continue
		}
		if cfg.Required[key] {
			continue
		}
		propSchema := cfg.Properties.GetOrNil(key)
		if pm, ok := propSchema.(*OrderedMap); ok {
			if def, hasDefault := pm.Get("default"); hasDefault {
				keyPath := fmt.Sprintf("%s.%s", path, key)
				copied, err := schemaReparer.copyJSONValue(def, keyPath, "default")
				if err != nil {
					p.recordSchemaErr(err)
					continue
				}
				obj.Set(key, copied)
				schemaReparer.recordLog("Inserted default value for missing property", keyPath)
			}
		}
	}
	return obj
}

// parseObjectKey mirrors _parse_object_key. It returns the parsed key and the
// rollback index (the position before the key was extracted). When the current
// character opens an array and no key has been set yet, it may fold that array
// into the previous key's value via mergeObjectArrayContinuation.
func (p *JSONParser) parseObjectKey(obj *OrderedMap, lastKey string) (string, int) {
	key := ""
	rollbackIndex := p.index
	p.context.set(ContextObjectKey)
	defer p.context.reset()
	for {
		ch, ok := p.charAt(0)
		if !ok {
			break
		}
		rollbackIndex = p.index
		if ch == '[' && key == "" && p.mergeObjectArrayContinuation(obj, lastKey) {
			continue
		}
		rawKey := p.parseString()
		key, _ = rawKey.(string)
		if key == "" {
			p.skipWhitespaces()
		}
		if key != "" || (key == "" && (p.charAtOr(0, 0) == ':' || p.charAtOr(0, 0) == '}')) {
			break
		}
	}
	return key, rollbackIndex
}

// mergeObjectArrayContinuation mirrors _merge_object_array_continuation.
// Consumes a following array and merges it into the value of lastKey.
func (p *JSONParser) mergeObjectArrayContinuation(obj *OrderedMap, lastKey string) bool {
	if lastKey == "" || p.strict {
		return false
	}
	prevAny, ok := obj.Get(lastKey)
	if !ok {
		return false
	}
	prev, isList := prevAny.([]any)
	if !isList {
		return false
	}

	p.index++ // consume '['
	newArray := p.parseArray("]")
	// parseArray always yields a []any (the Python isinstance(new_array, list)
	// guard is therefore always true here).

	var listLengths []int
	for _, item := range prev {
		if l, isL := item.([]any); isL {
			listLengths = append(listLengths, len(l))
		}
	}
	expectedLen := 0
	if len(listLengths) > 0 {
		equal := true
		for _, l := range listLengths {
			if l != listLengths[0] {
				equal = false
				break
			}
		}
		if equal {
			expectedLen = listLengths[0]
		}
	}

	if expectedLen > 0 {
		var tail []any
		for len(prev) > 0 {
			if _, isL := prev[len(prev)-1].([]any); !isL {
				tail = append(tail, prev[len(prev)-1])
				prev = prev[:len(prev)-1]
			} else {
				break
			}
		}
		if len(tail) > 0 {
			// reverse tail
			for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
				tail[i], tail[j] = tail[j], tail[i]
			}
			if len(tail)%expectedLen == 0 {
				p.log("While parsing an object we found row values without an inner array, grouping them into rows")
				for i := 0; i < len(tail); i += expectedLen {
					prev = append(prev, tail[i:i+expectedLen])
				}
			} else {
				prev = append(prev, tail...)
			}
		}
		if len(newArray) > 0 {
			allLists := true
			for _, item := range newArray {
				if _, isL := item.([]any); !isL {
					allLists = false
					break
				}
			}
			if allLists {
				p.log("While parsing an object we found additional rows, appending them without flattening")
				prev = append(prev, newArray...)
			} else {
				prev = append(prev, newArray)
			}
		}
	} else {
		if len(newArray) == 1 {
			if inner, isL := newArray[0].([]any); isL {
				prev = append(prev, inner...)
			} else {
				prev = append(prev, newArray...)
			}
		} else {
			prev = append(prev, newArray...)
		}
	}

	obj.Set(lastKey, prev)
	p.skipWhitespaces()
	if p.charAtOr(0, 0) == ',' {
		p.index++
	}
	p.skipWhitespaces()
	return true
}

// shouldSplitDuplicateObject mirrors _should_split_duplicate_object.
func (p *JSONParser) shouldSplitDuplicateObject(rollbackIndex int) bool {
	lookbackIdx := rollbackIndex - p.index - 1
	prevNonWhitespace, _ := p.charAt(lookbackIdx)
	for prevNonWhitespace != 0 && unicode.IsSpace(prevNonWhitespace) {
		lookbackIdx--
		prevNonWhitespace, _ = p.charAt(lookbackIdx)
	}
	keyStartChar, _ := p.charAt(rollbackIndex - p.index)
	nextNonWhitespace, _ := p.charAt(p.scrollWhitespaces(0))
	return !(isStringDelimiter(keyStartChar) && prevNonWhitespace == ',' && nextNonWhitespace == ':')
}

// splitObjectOnDuplicateKey mirrors _split_object_on_duplicate_key.
func (p *JSONParser) splitObjectOnDuplicateKey(rollbackIndex int) {
	p.index = rollbackIndex - 1
	newStr := make([]rune, 0, len(p.jsonStr)+1)
	newStr = append(newStr, p.jsonStr[:p.index+1]...)
	newStr = append(newStr, '{')
	newStr = append(newStr, p.jsonStr[p.index+1:]...)
	p.jsonStr = newStr
}

// repairEmptyObjectResult mirrors _repair_empty_object_result (with the salvage
// schema_set_object branch).
func (p *JSONParser) repairEmptyObjectResult(obj *OrderedMap, startIndex int, schema any, schemaReparer *SchemaRepairer) (bool, any) {
	if obj.Len() > 0 || p.index-startIndex <= 2 {
		return false, nil
	}
	if p.strict {
		p.log("Parsed object is empty but contains extra characters in strict mode, raising an error")
		p.fail("Parsed object is empty but contains extra characters in strict mode")
		return false, nil
	}
	classification, normalized := p.classifyEmptyObjectRepair(startIndex, schema, schemaReparer)
	switch classification {
	case "object":
		if normalized != nil {
			endIndex := p.index + 1
			if endIndex > len(p.jsonStr) {
				endIndex = len(p.jsonStr)
			}
			newStr := make([]rune, 0, len(p.jsonStr))
			newStr = append(newStr, p.jsonStr[:startIndex-1]...)
			newStr = append(newStr, []rune(*normalized)...)
			newStr = append(newStr, p.jsonStr[endIndex:]...)
			p.jsonStr = newStr
			p.index = startIndex
			closer := p.context.enter(ContextObjectKey)
			repaired := p.parseObjectSchema(schema, "$")
			closer()
			p.deferredContexts = append(p.deferredContexts, ContextObjectKey)
			return true, repaired
		}
	case "schema_set_object":
		p.log("Parsed object is empty but salvage schema expects an object, reparsing set-like members as null-valued object keys")
		p.index = startIndex
		closer := p.context.enter(ContextObjectKey)
		setItems := p.parseArray("]")
		closer()
		p.deferredContexts = append(p.deferredContexts, ContextObjectKey)
		if list, ok := any(setItems).([]any); ok {
			keyCandidates := []string{}
			allStrings := true
			for _, item := range list {
				if s, isStr := item.(string); isStr && s != "" {
					keyCandidates = append(keyCandidates, s)
				} else {
					allStrings = false
				}
			}
			if allStrings && len(keyCandidates) == len(list) {
				// Python dict.fromkeys(key_candidates): insertion order is the
				// candidate list order.
				m := NewOrderedMap()
				for _, k := range keyCandidates {
					m.Set(k, nil)
				}
				return true, m
			}
		}
		return true, setItems
	case "array":
		p.log("Parsed object is empty, we will try to parse this as an array instead")
		p.index = startIndex
		closer := p.context.enter(ContextObjectKey)
		repairedArray := p.parseArray("]")
		closer()
		p.deferredContexts = append(p.deferredContexts, ContextObjectKey)
		return true, repairedArray
	}
	return false, nil
}

// classifyEmptyObjectRepair mirrors _classify_empty_object_repair (with the
// salvage schema_set_object branch).
func (p *JSONParser) classifyEmptyObjectRepair(startIndex int, schema any, schemaReparer *SchemaRepairer) (string, *string) {
	end := p.index + 1
	if end > len(p.jsonStr) {
		end = len(p.jsonStr)
	}
	attempted := string(p.jsonStr[startIndex-1 : end])
	body := attempted[1:]
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimLeft(body, " \t\r\n")
	if body == "" {
		return "keep", nil
	}
	// 1:1 with Python: body.startswith('\\"') and '\\":' in body. Note the
	// contains-pattern is exactly 3 chars (backslash, quote, colon); written
	// as a concatenation so the literal cannot be misread as 4 chars.
	escapedKey := (strings.HasPrefix(body, "\\\"") && strings.Contains(body, "\\\""+":")) ||
		(strings.HasPrefix(body, "\\'") && strings.Contains(body, "\\'"+":"))
	if escapedKey {
		normalized := strings.ReplaceAll(attempted, "\\\"", "\"")
		normalized = strings.ReplaceAll(normalized, "\\'", "'")
		p.log("Parsed object is empty but the input starts like an escaped object key, normalizing and reparsing it as an object")
		out := normalized
		return "object", &out
	}
	stripped := strings.TrimLeft(stripCommentsForEmptyObjectClassification([]rune(body)), " \t\r\n")
	if stripped == "" {
		return "keep", nil
	}

	inQuote := rune(0)
	hasQuote := false
	backslashes := 0
	for _, char := range stripped {
		if char == '\\' {
			backslashes++
			continue
		}
		if hasQuote {
			if char == inQuote && backslashes%2 == 0 {
				hasQuote = false
			}
		} else if isStringDelimiter(char) && backslashes%2 == 0 {
			inQuote = char
			hasQuote = true
		} else if char == ':' && backslashes%2 == 0 {
			p.log("Parsed object is empty but the input still contains an object-style separator, keeping object repair")
			return "keep", nil
		}
		backslashes = 0
	}
	if schemaReparer != nil && schemaReparer.schemaRepairMode == "salvage" &&
		isMap(schema) && schemaReparer.IsObjectSchema(schema) && !schemaReparer.IsArraySchema(schema) {
		return "schema_set_object", nil
	}
	return "array", nil
}

// completeObjectParse mirrors _complete_object_parse (with schema finalization).
func (p *JSONParser) completeObjectParse(obj *OrderedMap, schema any, path string, schemaReparer *SchemaRepairer, schemaConfig *ObjectSchemaConfig) any {
	if !p.context.empty {
		if p.charAtOr(0, 0) == '}' && !p.context.currentIs(ContextObjectKey) && !p.context.currentIs(ContextObjectValue) {
			p.log("Found an extra closing brace that shouldn't be there, skipping it")
			p.index++
		}
		return obj
	}
	p.skipWhitespaces()
	if p.charAtOr(0, 0) == ',' {
		p.index++
		p.skipWhitespaces()
		if c, ok := p.charAt(0); ok && isStringDelimiter(c) && !p.strict {
			p.log("Found a comma and string delimiter after object closing brace, checking for additional key-value pairs")
			if more, ok2 := p.parseObjectSchema(schema, path).(*OrderedMap); ok2 {
				// Python obj.update(additional_obj): existing keys update in
				// place (position kept), new keys append in addition order.
				for _, k := range more.Keys() {
					obj.Set(k, more.GetOrNil(k))
				}
			}
		}
	}
	return p.finalizeObject(obj, schemaReparer, schemaConfig, path)
}

// stripCommentsForEmptyObjectClassification mirrors
// _strip_comments_for_empty_object_classification.
func stripCommentsForEmptyObjectClassification(body []rune) string {
	var stripped []rune
	inQuote := rune(0)
	hasQuote := false
	backslashes := 0
	index := 0
	n := len(body)
	for index < n {
		char := body[index]
		nextChar := rune(0)
		if index+1 < n {
			nextChar = body[index+1]
		}
		if char == '\\' {
			backslashes++
			stripped = append(stripped, char)
			index++
			continue
		}
		if hasQuote {
			stripped = append(stripped, char)
			if char == inQuote && backslashes%2 == 0 {
				hasQuote = false
			}
			backslashes = 0
			index++
			continue
		}
		if isStringDelimiter(char) && backslashes%2 == 0 {
			inQuote = char
			hasQuote = true
			stripped = append(stripped, char)
			backslashes = 0
			index++
			continue
		}
		backslashes = 0
		if char == '#' {
			index++
			for index < n && body[index] != '\n' && body[index] != '\r' {
				index++
			}
			continue
		}
		if char == '/' && nextChar == '/' {
			index += 2
			for index < n && body[index] != '\n' && body[index] != '\r' {
				index++
			}
			continue
		}
		if char == '/' && nextChar == '*' {
			index += 2
			for index < n-1 && !(body[index] == '*' && body[index+1] == '/') {
				index++
			}
			index += 2
			if index > n {
				index = n
			}
			continue
		}
		stripped = append(stripped, char)
		index++
	}
	return string(stripped)
}
