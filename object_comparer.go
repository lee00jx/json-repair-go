package json_repair

// ObjectComparer mirrors the Python utils.object_comparer module. It is used
// by the top-level merge logic to decide whether two top-level values are the
// "same" object (update semantics).

// isSameObject recursively compares two values, requiring their types to
// match (Python `type(a) is type(b)` semantics) as well as their structure.
func isSameObject(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case *OrderedMap:
		bv, ok := b.(*OrderedMap)
		if !ok || av.Len() != bv.Len() {
			return false
		}
		for _, k := range av.keys {
			vb, ok2 := bv.values[k]
			if !ok2 || !isSameObject(av.values[k], vb) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !isSameObject(av[i], bv[i]) {
				return false
			}
		}
		return true
	case string:
		// atomic: type match only (mirrors Python `type(a) is type(b)`)
		_, ok := b.(string)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	case int64:
		_, ok := b.(int64)
		return ok
	case uint64:
		_, ok := b.(uint64)
		return ok
	case float64:
		_, ok := b.(float64)
		return ok
	default:
		return false
	}
}

// isStrictlyEmpty returns true for empty containers (string, array, object);
// false for scalars like nil, 0, false.
func isStrictlyEmpty(value any) bool {
	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case *OrderedMap:
		return v.Len() == 0
	default:
		return false
	}
}
