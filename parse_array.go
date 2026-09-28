package json_repair

import (
	"fmt"
	"unicode"
)

// parseArray is the non-schema entry point.
func (p *JSONParser) parseArray(closing string) []any {
	return p.parseArraySchema(nil, "$", closing)
}

// resolveArrayItemSchema mirrors Python _resolve_array_item_schema: returns the
// item schema for index idx and whether the item should be dropped (when the
// schema forbids extra items). Bad (non-dict/bool) schemas record a schema error.
func (p *JSONParser) resolveArrayItemSchema(cfg *ArraySchemaConfig, idx int) (any, bool) {
	if cfg == nil {
		return nil, false
	}
	var itemSchema any
	dropItem := false
	switch items := cfg.ItemsSchema.(type) {
	case []any:
		if idx < len(items) {
			raw := items[idx]
			if raw != nil && !isSchemaType(raw) {
				p.recordSchemaErr(schemaDefErr("Schema must be an object."))
				return nil, false
			}
			itemSchema = raw
		} else if b, ok := cfg.AdditionalItems.(bool); ok && !b {
			dropItem = true
		} else if _, ok := cfg.AdditionalItems.(*OrderedMap); ok {
			itemSchema = cfg.AdditionalItems
		} else {
			itemSchema = true
		}
	case *OrderedMap:
		itemSchema = items
	default:
		itemSchema = true
	}
	return itemSchema, dropItem
}

func isSchemaType(v any) bool {
	switch v.(type) {
	case *OrderedMap, bool:
		return true
	}
	return false
}

// parseArraySchema mirrors Python parse_array: parses comma-separated items and,
// when a schema repairer is active, applies per-item schemas (coercions, drops).
func (p *JSONParser) parseArraySchema(schema any, path string, closing string) []any {
	closingRune := rune(closing[0])
	schemaReparer, _, schemaConfig, rerr := resolveParserArraySchema(p.schemaRepairer, schema)
	if rerr != nil {
		p.recordSchemaErr(rerr)
	}
	salvageMode := schemaReparer != nil && schemaReparer.schemaRepairMode == "salvage"
	arr := []any{}
	arrayIsObjectValue := p.context.currentIs(ContextObjectValue)
	closedBeforeParentMember := false
	defer p.context.enter(ContextArray)()

	p.skipWhitespaces()
	ch, ok := p.charAt(0)
	idx := 0
	for ok && ch != closingRune && ch != '}' {
		itemSchema, dropItem := p.resolveArrayItemSchema(schemaConfig, idx)
		itemPath := fmt.Sprintf("%s[%d]", path, idx)
		activeReparer := (*SchemaRepairer)(nil)
		if schemaReparer != nil && !dropItem && !salvageMode {
			activeReparer = schemaReparer
		}

		var value any
		if isStringDelimiter(ch) {
			// A string followed by ':' is often a missing object start.
			i := 1
			i = p.skipToCharacter([]rune{ch}, i)
			i = p.scrollWhitespaces(i + 1)
			if p.charAtIs(i, ':') {
				if arrayIsObjectValue && len(arr) > 0 && allScalars(arr) {
					p.log("Closed unclosed array before object member after scalar items")
					closedBeforeParentMember = true
					break
				}
				if activeReparer != nil {
					value = p.parseObjectSchema(itemSchema, itemPath)
					repaired, err := activeReparer.RepairValue(value, itemSchema, itemPath)
					if err != nil {
						p.recordSchemaErr(err)
					} else {
						value = repaired
					}
				} else {
					value = p.parseObject()
				}
			} else {
				value = p.parseString()
				if activeReparer != nil {
					repaired, err := activeReparer.RepairValue(value, itemSchema, itemPath)
					if err != nil {
						p.recordSchemaErr(err)
					} else {
						value = repaired
					}
				}
			}
		} else {
			if activeReparer != nil {
				value = p.parseJSONSchema(itemSchema, itemPath, true, false)
			} else {
				value = p.parseJSON()
			}
		}

		next, _ := p.charAt(0)
		switch {
		case isStrictlyEmpty(value) && next != closingRune && next != ',':
			p.index++
		case value == "..." && charBeforeIs(p, '.'):
			p.log("While parsing an array, found a stray '...'; ignoring it")
		case !dropItem:
			arr = append(arr, value)
		default:
			if schemaReparer != nil {
				schemaReparer.recordLog("Dropped extra array item not covered by schema", itemPath)
			}
		}

		idx++
		ch, ok = p.charAt(0)
		for ok && ch != closingRune && (unicode.IsSpace(ch) || ch == ',') {
			p.index++
			ch, ok = p.charAt(0)
		}
	}

	if ok && ch != closingRune {
		p.log(fmt.Sprintf("While parsing an array we missed the closing %s, ignoring it", closing))
	}
	if !closedBeforeParentMember {
		p.index++
	}
	return arr
}

// allScalars reports whether every element is a scalar (no object or array),
// mirroring `all(not isinstance(item, (dict, list)) for item in arr)`.
func allScalars(arr []any) bool {
	for _, item := range arr {
		switch item.(type) {
		case *OrderedMap, map[string]any, []any:
			return false
		}
	}
	return true
}

func charBeforeIs(p *JSONParser, r rune) bool {
	c, ok := p.charAt(-1)
	return ok && c == r
}
