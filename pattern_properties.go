package json_repair

import "strings"

// unsupportedRegexTokens mirrors Python _UNSUPPORTED_REGEX_TOKENS: if any of
// these appears in the literal part of a patternProperties key, the pattern is
// treated as unsupported and skipped (we intentionally do NOT run user regex).
var unsupportedRegexTokens = []string{".", "^", "$", "*", "+", "?", "{", "}", "[", "]", "|", "(", ")", "\\"}

// matchPatternProperties mirrors Python match_pattern_properties: a safe
// literal + anchor subset. Patterns are evaluated in schema insertion order
// (Python iterates the schema dict in order), so the first match and the
// matched-schemas order are deterministic and 1:1 with Python. Returns the
// matched schemas (in pattern order) and the list of unsupported pattern
// strings.
func matchPatternProperties(patternProperties *OrderedMap, key string) ([]any, []string) {
	if patternProperties.Len() == 0 {
		return nil, nil
	}
	matched := []any{}
	unsupported := []string{}
	for _, pattern := range patternProperties.Keys() {
		schema := patternProperties.values[pattern]
		anchoredStart := strings.HasPrefix(pattern, "^")
		anchoredEnd := strings.HasSuffix(pattern, "$")
		lit := pattern
		if anchoredStart {
			lit = lit[1:]
		}
		if anchoredEnd && len(lit) > 0 {
			lit = lit[:len(lit)-1]
		}

		isUnsupported := false
		for _, token := range unsupportedRegexTokens {
			if strings.Contains(lit, token) {
				isUnsupported = true
				break
			}
		}
		if isUnsupported {
			unsupported = append(unsupported, pattern)
			continue
		}

		var isMatch bool
		switch {
		case anchoredStart && anchoredEnd:
			isMatch = key == lit
		case anchoredStart:
			isMatch = strings.HasPrefix(key, lit)
		case anchoredEnd:
			isMatch = strings.HasSuffix(key, lit)
		default:
			isMatch = strings.Contains(key, lit)
		}
		if isMatch {
			matched = append(matched, schema)
		}
	}
	return matched, unsupported
}
