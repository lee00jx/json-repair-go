package json_repair

import "fmt"

// ObjectSchemaConfig mirrors Python parser_schema.ObjectSchemaConfig. The
// *OrderedMap properties keep schema insertion order; PropertyKeys /
// PatternKeys are that order as slices so consumers can iterate in Python
// dict order (1:1 key-order semantics, DECISIONS D-006).
type ObjectSchemaConfig struct {
	Properties           *OrderedMap
	PropertyKeys         []string // insertion order of Properties
	PatternProperties    *OrderedMap
	PatternKeys          []string // insertion order of PatternProperties
	AdditionalProperties any      // nil if absent, bool or *OrderedMap if present
	Required             map[string]bool
}

// ArraySchemaConfig mirrors Python parser_schema.ArraySchemaConfig.
type ArraySchemaConfig struct {
	ItemsSchema     any // nil if absent; *OrderedMap, []any (tuple), bool
	AdditionalItems any // nil if absent; bool or *OrderedMap
}

func objectSchemaConfig(schema *OrderedMap) *ObjectSchemaConfig {
	var properties *OrderedMap
	if p, ok := schema.Get("properties"); ok {
		if pm, isMap := p.(*OrderedMap); isMap {
			properties = pm
		}
	}
	if properties == nil {
		properties = NewOrderedMap()
	}
	var patternProperties *OrderedMap
	if pp, ok := schema.Get("patternProperties"); ok {
		if ppm, isMap := pp.(*OrderedMap); isMap {
			patternProperties = ppm
		}
	}
	if patternProperties == nil {
		patternProperties = NewOrderedMap()
	}
	required := map[string]bool{}
	if req, ok := schema.Get("required"); ok {
		if list, isList := req.([]any); isList {
			for _, r := range list {
				if s, ok := r.(string); ok {
					required[s] = true
				}
			}
		}
	}
	return &ObjectSchemaConfig{
		Properties:           properties,
		PropertyKeys:         properties.Keys(),
		PatternProperties:    patternProperties,
		PatternKeys:          patternProperties.Keys(),
		AdditionalProperties: schema.GetOrNil("additionalProperties"),
		Required:             required,
	}
}

func arraySchemaConfig(schema *OrderedMap) *ArraySchemaConfig {
	return &ArraySchemaConfig{
		ItemsSchema:     schema.GetOrNil("items"),
		AdditionalItems: schema.GetOrNil("additionalItems"),
	}
}

// resolveParserObjectSchema mirrors Python resolve_parser_object_schema.
func resolveParserObjectSchema(repairer *SchemaRepairer, schema any) (*SchemaRepairer, any, *ObjectSchemaConfig, error) {
	if repairer == nil || schema == nil || schema == true {
		return nil, schema, nil, nil
	}
	resolved, err := repairer.ResolveSchema(schema)
	if err != nil {
		return nil, schema, nil, err
	}
	if resolved == false {
		return nil, schema, nil, fmt.Errorf("Schema does not allow any values.")
	}
	if resolved == true || !repairer.IsObjectSchema(resolved) {
		return nil, schema, nil, nil
	}
	rSchema, _ := resolved.(*OrderedMap)
	return repairer, resolved, objectSchemaConfig(rSchema), nil
}

// resolveParserArraySchema mirrors Python resolve_parser_array_schema.
func resolveParserArraySchema(repairer *SchemaRepairer, schema any) (*SchemaRepairer, any, *ArraySchemaConfig, error) {
	if repairer == nil || schema == nil || schema == true {
		return nil, schema, nil, nil
	}
	resolved, err := repairer.ResolveSchema(schema)
	if err != nil {
		return nil, schema, nil, err
	}
	if resolved == false {
		return nil, schema, nil, fmt.Errorf("Schema does not allow any values.")
	}
	if resolved == true || !repairer.IsArraySchema(resolved) {
		return nil, schema, nil, nil
	}
	rSchema, _ := resolved.(*OrderedMap)
	return repairer, resolved, arraySchemaConfig(rSchema), nil
}
