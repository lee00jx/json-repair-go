package json_repair

// JSONValue is the type alias for any parsed JSON value, mirroring the
// Python JSONReturnType (dict | list | str | float | int | bool | None).
//
// Type mapping (DECISIONS D-002, D-006):
//   - JSON object  -> *OrderedMap (insertion-ordered, Python dict semantics)
//   - JSON array   -> []any
//   - JSON string  -> string
//   - JSON bool    -> bool
//   - JSON integer -> int64
//   - JSON number  -> float64
//   - JSON null    -> nil
type JSONValue = any

// StringDelimiters are the quote characters recognised as string boundaries.
// The two smart quotes map onto each other (open U+201C / close U+201D).
var StringDelimiters = []rune{'"', '\'', '\u201c', '\u201d'}

// isStringDelimiter reports whether r is one of the recognised string delimiters.
func isStringDelimiter(r rune) bool {
	for _, d := range StringDelimiters {
		if r == d {
			return true
		}
	}
	return false
}

// matchingStringDelimiter returns the closing delimiter for an opening one.
func matchingStringDelimiter(delimiter rune) rune {
	if delimiter == '\u201c' {
		return '\u201d'
	}
	return delimiter
}
