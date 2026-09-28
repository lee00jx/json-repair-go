package json_repair

import "sort"

// OrderedMap is the Go counterpart of a Python dict: an insertion-ordered
// string-keyed map. CPython 3.7+ dicts preserve insertion order and
// json.dumps emits keys in that order, so the repairer builds and returns
// OrderedMap (not map[string]any) to keep object key order 1:1 with Python
// (DECISIONS D-006, closed by this type).
//
// Dict semantics mirrored from Python:
//   - Set on an existing key updates the value in place (position unchanged);
//   - Set on a new key appends the key at the end;
//   - Delete removes the key; a later Set appends it again at the end;
//   - iteration (Keys/Values and internal m.keys) is insertion order.
//
// The zero value is NOT usable; construct with NewOrderedMap so that an empty
// map marshals as {} (a nil values map would still marshal as {}, but
// Get/Set on a zero value would panic).
type OrderedMap struct {
	keys   []string
	values map[string]any
}

// NewOrderedMap returns an empty ordered map.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{values: map[string]any{}}
}

// Set stores value under key, mirroring Python `d[key] = value`: existing
// keys are updated in place, new keys are appended to the key order.
func (m *OrderedMap) Set(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Get returns the value for key and whether it was present (Python `d[key]`
// plus a `key in d` check).
func (m *OrderedMap) Get(key string) (any, bool) {
	v, ok := m.values[key]
	return v, ok
}

// Has reports whether key is present (Python `key in d`).
func (m *OrderedMap) Has(key string) bool {
	_, ok := m.values[key]
	return ok
}

// Delete removes key (Python `del d[key]`, as a no-op when absent).
func (m *OrderedMap) Delete(key string) {
	if _, exists := m.values[key]; !exists {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

// Len returns the number of keys (Python `len(d)`).
func (m *OrderedMap) Len() int { return len(m.keys) }

// Keys returns the keys in insertion order (Python `list(d.keys())`).
func (m *OrderedMap) Keys() []string {
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

// Values returns the values in key insertion order (Python `list(d.values())`).
func (m *OrderedMap) Values() []any {
	out := make([]any, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, m.values[k])
	}
	return out
}

// Clone returns a shallow copy: a new key-order/value container sharing the
// (deep-copied) values of the original. Use deepCopyJSON for a deep copy.
func (m *OrderedMap) Clone() *OrderedMap {
	out := &OrderedMap{
		keys:   make([]string, len(m.keys)),
		values: make(map[string]any, len(m.values)),
	}
	copy(out.keys, m.keys)
	for k, v := range m.values {
		out.values[k] = v
	}
	return out
}

// GetOrNil mirrors Python `dict.get(key)`: the value, or nil when the key is
// absent (Go's Get would need an ok-check at every call site otherwise).
func (m *OrderedMap) GetOrNil(key string) any {
	if v, ok := m.Get(key); ok {
		return v
	}
	return nil
}

// ToPlain converts to an unordered map[string]any view (one copy). Use when
// handing a value to APIs that require a plain map.
func (m *OrderedMap) ToPlain() map[string]any {
	out := make(map[string]any, len(m.values))
	for k, v := range m.values {
		out[k] = v
	}
	return out
}

// MarshalJSON serializes in insertion order (compact separators,
// ensure_ascii off — matching Dump's default form). This is what makes
// encoding/json consumers (gojsonschema validation, json.Marshal of a
// returned value) emit Python-order keys.
func (m *OrderedMap) MarshalJSON() ([]byte, error) {
	s, err := Dump(m, DumpOptions{})
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// OrderedFromPlain converts a plain (orderless) map into an OrderedMap. A
// plain map carries no order, so keys are sorted: the deterministic
// convention of this project for orderless input (DECISIONS D-006).
func OrderedFromPlain(in map[string]any) *OrderedMap {
	m := NewOrderedMap()
	if in == nil {
		return m
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.keys = append(m.keys, k)
		m.values[k] = in[k]
	}
	return m
}

// toOrderedSchema accepts a JSON Schema node that is already an ordered tree
// (*OrderedMap, the production form after SchemaFromInput /
// NewSchemaRepairer normalization) or a plain map (converted once, deep, with
// sorted keys — the deterministic convention for orderless input). Tests and
// library callers may pass plain map literals without normalizing first.
func toOrderedSchema(v any) (*OrderedMap, bool) {
	switch t := v.(type) {
	case *OrderedMap:
		return t, true
	case map[string]any:
		return orderedMapFromPlainDeep(t), true
	}
	return nil, false
}

// orderedMapFromPlainDeep deep-converts a plain object (and its nested
// values) to an ordered tree: objects become OrderedMap with sorted keys,
// arrays keep element order.
func orderedMapFromPlainDeep(root map[string]any) *OrderedMap {
	out := NewOrderedMap()
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.keys = append(out.keys, k)
		out.values[k] = orderedFromPlainDeep(root[k])
	}
	return out
}

// orderedFromPlainDeep deep-converts a plain JSON value tree to an ordered
// tree (objects become OrderedMap with sorted keys, arrays keep order).
func orderedFromPlainDeep(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return orderedMapFromPlainDeep(t)
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = orderedFromPlainDeep(el)
		}
		return out
	default:
		return v
	}
}

// toPlainValue deep-converts an ordered JSON value tree back to plain values
// (OrderedMap -> map[string]any, recursively), for order-insensitive
// comparisons (Python `==` on dicts ignores key order).
func toPlainValue(v any) any {
	switch t := v.(type) {
	case *OrderedMap:
		out := make(map[string]any, t.Len())
		for _, k := range t.keys {
			out[k] = toPlainValue(t.values[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = toPlainValue(el)
		}
		return out
	default:
		return v
	}
}
