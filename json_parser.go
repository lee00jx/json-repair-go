package json_repair

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// RepairLog is one entry in the repair log, mirroring the Python log dicts
// with keys "text" and "context".
type RepairLog struct {
	Text    string
	Context string
}

// JSONParser is the core recursive-descent parser. It keeps a rune cursor
// (index) over the input and an object-member context stack, then dispatches
// on the first character to the object/array/string/number/comment parsers.
//
// The input is held as []rune so that indexing matches Python's Unicode
// code-point semantics (smart quotes, CJK, etc.), which is required for 1:1
// behavioural parity.
type JSONParser struct {
	jsonStr             []rune
	index               int
	context             *JsonContext
	deferredContexts    []ContextValue
	logging             bool
	logger              []RepairLog
	streamStable        bool
	strict              bool
	tryValidJsonSuffix  bool
	hasTriedValidSuffix bool
	// strictErr is set when strict mode encounters a structural violation.
	strictErr error
	// schemaRepairer is set by ParseWithSchema for schema-guided parsing.
	schemaRepairer *SchemaRepairer
	// lastParseFoundValue records whether the most recent parseJSONSchema (with
	// record_top_level_value) found a value; used by the salvage top-level loop.
	lastParseFoundValue bool
	// schemaErr holds the first schema-guided ValueError-equivalent raised during
	// schema parsing (record-and-continue; RepairJSON surfaces it).
	schemaErr error
}

func newJSONParser(
	jsonStr string,
	logging bool,
	streamStable bool,
	strict bool,
	tryValidJsonSuffix bool,
) *JSONParser {
	return &JSONParser{
		jsonStr:            []rune(jsonStr),
		context:            newJsonContext(),
		logging:            logging,
		streamStable:       streamStable,
		strict:             strict,
		tryValidJsonSuffix: tryValidJsonSuffix,
	}
}

// ---- cursor helpers ----

func (p *JSONParser) charAt(count int) (rune, bool) {
	i := p.index + count
	if i < 0 || i >= len(p.jsonStr) {
		return 0, false
	}
	return p.jsonStr[i], true
}

// charAtIs reports whether the character at index+count equals target
// (false when out of range), mirroring Python `get_char_at(count) == target`.
func (p *JSONParser) charAtIs(count int, target rune) bool {
	ch, ok := p.charAt(count)
	return ok && ch == target
}

// charAtOr returns the character at index+count, or fallback when out of range.
// It mirrors Python's `(self.get_char_at() or <fallback>)` idiom used to treat
// end-of-input as a specific sentinel (e.g. '}').
func (p *JSONParser) charAtOr(count int, fallback rune) rune {
	ch, ok := p.charAt(count)
	if !ok {
		return fallback
	}
	return ch
}

func (p *JSONParser) skipWhitespaces() {
	for {
		ch, ok := p.charAt(0)
		if !ok || !unicode.IsSpace(ch) {
			break
		}
		p.index++
	}
}

// scrollWhitespaces returns the offset from index to the next non-whitespace
// character without moving index.
func (p *JSONParser) scrollWhitespaces(idx int) int {
	for {
		ch, ok := p.charAt(idx)
		if !ok || !unicode.IsSpace(ch) {
			break
		}
		idx++
	}
	return idx
}

// skipToCharacter advances from index+idx to the next *unescaped* target rune
// and returns the offset from index. If not found, returns distance to end.
func (p *JSONParser) skipToCharacter(targets []rune, idx int) int {
	i := p.index + idx
	n := len(p.jsonStr)
	backslashes := 0
	for i < n {
		ch := p.jsonStr[i]
		if ch == '\\' {
			backslashes++
			i++
			continue
		}
		if inRunes(targets, ch) && backslashes%2 == 0 {
			return i - p.index
		}
		backslashes = 0
		i++
	}
	return n - p.index
}

func (p *JSONParser) log(text string) {
	if !p.logging {
		return
	}
	window := 10
	start := p.index - window
	if start < 0 {
		start = 0
	}
	end := p.index + window
	if end > len(p.jsonStr) {
		end = len(p.jsonStr)
	}
	p.logger = append(p.logger, RepairLog{Text: text, Context: string(p.jsonStr[start:end])})
}

// fail records a strict-mode error. Only the first error is kept.
func (p *JSONParser) fail(msg string) {
	if p.strictErr == nil {
		p.strictErr = fmt.Errorf("%s", msg)
	}
}

// ---- top-level ----

// Parse parses a whole input and returns the resulting value (or a []any of
// top-level values when multiple were found).
func (p *JSONParser) Parse() any {
	return p.parseTopLevel(p.parseJSON)
}

// ParseWithSchema mirrors Python parse_with_schema: enables schema guidance for
// all nested values. In salvage mode it returns the first schema-valid
// top-level fragment (skipping ones that fail repair/validation).
func (p *JSONParser) ParseWithSchema(repairer *SchemaRepairer, schema any) (result any) {
	defer func() {
		if rec := recover(); rec != nil {
			if sa, ok := rec.(schemaAbort); ok {
				p.schemaErr = sa.err
				result = ""
				return
			}
			panic(rec)
		}
	}()
	p.schemaRepairer = repairer
	if repairer.schemaRepairMode == "salvage" {
		return p.parseTopLevelSalvageWithSchema(repairer, schema)
	}
	return p.parseTopLevel(func() any { return p.parseJSONSchema(schema, "$", true, false) })
}

// parseTopLevelSalvageWithSchema mirrors Python _parse_top_level_salvage_with_schema.
func (p *JSONParser) parseTopLevelSalvageWithSchema(repairer *SchemaRepairer, schema any) any {
	var lastErr error
	for p.index < len(p.jsonStr) {
		p.context.clear()
		p.deferredContexts = nil
		value := p.parseJSONSchema(schema, "$", false, true)
		if !p.lastParseFoundValue {
			break
		}
		repaired, rerr := repairer.RepairValue(value, schema, "$")
		var verr error
		if rerr != nil {
			verr = rerr
		} else {
			verr = repairer.validate(repaired, schema)
		}
		if verr != nil {
			lastErr = verr
			if p.index >= len(p.jsonStr) {
				break
			}
			p.log("Skipped top-level fragment that did not match schema while salvaging")
			continue
		}
		return repaired
	}
	if lastErr != nil {
		p.recordSchemaErr(lastErr)
		return ""
	}
	return ""
}

// parseTopLevel consolidates multiple sequential top-level values with the
// Python update/merge semantics.
func (p *JSONParser) parseTopLevel(parseElement func() any) any {
	first := parseElement()
	if p.index < len(p.jsonStr) {
		p.log("The parser returned early, checking if there's more json elements")
		if p.strict {
			p.fail("Multiple top-level JSON elements found in strict mode")
		}
		acc := []any{first}
		for p.index < len(p.jsonStr) {
			p.context.clear()
			p.deferredContexts = nil
			isComma := p.nextTopLevelValueIsCommaSeparated()
			el := parseElement()
			if isTruthyJSON(el) {
				last := acc[len(acc)-1]
				if !isComma && isSameObject(last, el) {
					acc = acc[:len(acc)-1]
				} else if isStrictlyEmpty(last) {
					acc = acc[:len(acc)-1]
				}
				acc = append(acc, el)
			} else {
				p.index++
			}
		}
		if len(acc) == 1 {
			p.log("There were no more elements, returning the element without the array")
			return acc[0]
		}
		return acc
	}
	return first
}

func (p *JSONParser) nextTopLevelValueIsCommaSeparated() bool {
	idx := p.scrollWhitespaces(0)
	if ch, ok := p.charAt(idx); ok && ch == ',' {
		return true
	}
	idx = p.index - 1
	for idx >= 0 && unicode.IsSpace(p.jsonStr[idx]) {
		idx--
	}
	return idx >= 0 && p.jsonStr[idx] == ','
}

// ---- dispatch ----

// parseJSON is the non-schema entry point; it dispatches with no schema.
func (p *JSONParser) parseJSON() any {
	return p.parseJSONSchema(nil, "$", true, false)
}

// parseJSONSchema mirrors Python parse_json: dispatches on the first character
// and, when a schema repairer is active, guides nested object/array parsing and
// finalizes the value against the schema via repair_value.
func (p *JSONParser) parseJSONSchema(schema any, path string, finalizeSchema bool, recordTopLevelValue bool) any {
	if recordTopLevelValue {
		p.lastParseFoundValue = false
	}
	// Replay deferred contexts: the Python parser records these (e.g. after an
	// empty-object was reparsed as an array/set) so that the next parse_json
	// happens inside the OBJECT_KEY context.
	if len(p.deferredContexts) > 0 {
		deferred := p.deferredContexts
		p.deferredContexts = nil
		closer := p.context.enterValues(deferred)
		defer closer()
		return p.parseJSONSchema(schema, path, finalizeSchema, recordTopLevelValue)
	}
	repairer, schema := p.resolveSchemaForParse(schema)
	for {
		ch, ok := p.charAt(0)
		if !ok {
			return ""
		}
		if p.tryValidJsonSuffix && (ch == '{' || ch == '[') && p.initialContainerHasNonCommaTrailingContent() {
			if value, ok2 := p.tryParseValidJsonValue(); ok2 {
				return p.finalizeParsedValue(value, repairer, schema, path, finalizeSchema, recordTopLevelValue)
			}
		}
		switch {
		case ch == '{':
			p.index++
			var value any
			if repairer != nil {
				value = p.parseObjectSchema(schema, path)
			} else {
				value = p.parseObject()
			}
			return p.finalizeParsedValue(value, repairer, schema, path, finalizeSchema, recordTopLevelValue)
		case ch == '[':
			p.index++
			var value any
			if repairer != nil {
				value = p.parseArraySchema(schema, path, "]")
			} else {
				value = p.parseArray("]")
			}
			return p.finalizeParsedValue(value, repairer, schema, path, finalizeSchema, recordTopLevelValue)
		case ch == '(':
			if !p.context.empty || p.topLevelParenthesizedCanStartValue() {
				var value any
				if repairer != nil {
					value = p.parseParenthesizedSchema(schema, path)
				} else {
					value = p.parseParenthesized()
				}
				return p.finalizeParsedValue(value, repairer, schema, path, finalizeSchema, recordTopLevelValue)
			}
			p.index++
		case !p.context.empty && (isStringDelimiter(ch) || unicode.IsLetter(ch)):
			return p.finalizeParsedValue(p.parseString(), repairer, schema, path, finalizeSchema, recordTopLevelValue)
		case !p.context.empty && (unicode.IsDigit(ch) || ch == '-' || ch == '.'):
			return p.finalizeParsedValue(p.parseNumber(), repairer, schema, path, finalizeSchema, recordTopLevelValue)
		case ch == '#' || ch == '/':
			return p.finalizeParsedValue(p.parseComment(false), repairer, schema, path, finalizeSchema, false)
		default:
			p.index++
		}
	}
}

// tryParseValidJsonValue mirrors _try_parse_valid_json_value: when enabled, at a
// top-level container start, and in an empty context, decode a single JSON value
// directly. This is both a fast path and a correctness path — it preserves
// content the character scanner can mangle (e.g. a JSON-escaped newline followed
// by a comma inside a nested array/object). ok is true only on a successful
// strict decode; object keys keep insertion order and numbers are normalized
// to int64/float64 (DECISIONS D-002, D-006). The decoded value's exact end
// offset is used to advance the cursor, like Python raw_decode.
func (p *JSONParser) tryParseValidJsonValue() (any, bool) {
	if !p.tryValidJsonSuffix || p.hasTriedValidSuffix || !p.context.empty {
		return nil, false
	}
	p.hasTriedValidSuffix = true
	v, byteEnd, err := decodeOrderedStrict(string(p.jsonStr[p.index:]))
	if err != nil {
		return nil, false
	}
	runesConsumed, byteCount := 0, 0
	for _, r := range p.jsonStr[p.index:] {
		if byteCount >= byteEnd {
			break
		}
		byteCount += len(string(r))
		runesConsumed++
	}
	p.index += runesConsumed
	return v, true
}

// initialContainerHasNonCommaTrailingContent mirrors
// _initial_container_has_non_comma_trailing_content. At index 0 it verifies the
// first balanced top-level container is not immediately followed by a comma (a
// trailing comma would mean more top-level values follow, which must not be
// grabbed by a single raw decode). At any other index it returns true.
func (p *JSONParser) initialContainerHasNonCommaTrailingContent() bool {
	if p.index != 0 {
		return true
	}
	var containers []rune
	inString, escaped := false, false
	for i, ch := range p.jsonStr {
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
		case ch == '{' || ch == '[':
			containers = append(containers, ch)
		case ch == '}' || ch == ']':
			if len(containers) == 0 {
				return false
			}
			top := containers[len(containers)-1]
			containers = containers[:len(containers)-1]
			if (top == '{' && ch != '}') || (top == '[' && ch != ']') {
				return false
			}
			if len(containers) == 0 {
				trailing := i + 1
				for trailing < len(p.jsonStr) && unicode.IsSpace(p.jsonStr[trailing]) {
					trailing++
				}
				return trailing < len(p.jsonStr) && p.jsonStr[trailing] != ','
			}
		}
	}
	return false
}

// resolveSchemaForParse mirrors Python _resolve_schema_for_parse: returns the
// active repairer (nil when schema is None/True) and the resolved schema. A
// schema that resolves to False records "Schema does not allow any values".
func (p *JSONParser) resolveSchemaForParse(schema any) (*SchemaRepairer, any) {
	if p.schemaRepairer == nil || schema == nil || schema == true {
		return nil, schema
	}
	resolved, err := p.schemaRepairer.ResolveSchema(schema)
	if err != nil {
		p.recordSchemaErr(err)
		return nil, schema
	}
	if resolved == true {
		return nil, schema
	}
	if resolved == false {
		p.recordSchemaErr(errors.New("Schema does not allow any values."))
		return nil, schema
	}
	return p.schemaRepairer, resolved
}

// finalizeParsedValue mirrors Python _finalize_parsed_value.
func (p *JSONParser) finalizeParsedValue(value any, repairer *SchemaRepairer, schema any, path string, finalizeSchema bool, recordTopLevelValue bool) any {
	if recordTopLevelValue {
		p.lastParseFoundValue = true
	}
	if repairer == nil || !finalizeSchema {
		return value
	}
	repaired, err := repairer.RepairValue(value, schema, path)
	if err != nil {
		p.recordSchemaErr(err)
		return value
	}
	return repaired
}

// recordSchemaErr stores the first schema error and aborts schema-guided
// parsing (mirrors Python raising ValueError from nested parse helpers).
func (p *JSONParser) recordSchemaErr(err error) {
	if err == nil {
		return
	}
	if p.schemaErr == nil {
		p.schemaErr = err
	}
	panic(schemaAbort{err})
}

// schemaAbort is recovered at ParseWithSchema, mirroring Python's ValueError
// abort of schema-guided parse.
type schemaAbort struct{ err error }

// ---- number ----

func isNumberChar(r rune) bool {
	return strings.ContainsRune("0123456789-.eE/,_", r)
}

func (p *JSONParser) parseNumber() any {
	var numberStr []rune
	ch, ok := p.charAt(0)
	isArray := p.context.currentIs(ContextArray)
	for ok && isNumberChar(ch) && (!isArray || ch != ',') {
		if ch != '_' {
			numberStr = append(numberStr, ch)
		}
		p.index++
		ch, ok = p.charAt(0)
	}
	s := string(numberStr)
	next, okNext := p.charAt(0)
	if okNext && unicode.IsLetter(next) {
		// this was a string instead, sorry
		p.index -= len(numberStr)
		return p.parseString()
	}
	if len(s) > 0 && strings.ContainsRune("-eE/,", rune(s[len(s)-1])) {
		s = s[:len(s)-1]
		p.index--
	}
	switch {
	case strings.Contains(s, ","):
		return s
	case strings.ContainsAny(s, ".eE"):
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return f
	default:
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return i
		}
		// Integer literals outside the int64 range fall back to float64,
		// matching the stdlib fast path (decodeOrderedValue) which also yields
		// float64 for out-of-range integers (DECISIONS D-002). Returning a
		// string here was the source of a skip_json_loads inconsistency: the
		// same value decoded to float64 on the fast path but string here.
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	}
}

// ---- comment ----

func (p *JSONParser) parseComment(recordTopLevelValue bool) any {
	for {
		ch, ok := p.charAt(0)
		termination := []rune{'\n', '\r'}
		// Mirror Python exactly: all three checks test membership in the whole
		// context STACK (`ContextValues.X in self.context.context`), not just the
		// innermost value. Using `currentIs` here dropped trailing keys: with a
		// nested object the stack holds both OBJECT_VALUE and OBJECT_KEY, so the
		// '#' line comment must stop at '}' (earlier than the ':' after the next
		// key) to preserve that key.
		if p.context.in(ContextArray) {
			termination = append(termination, ']')
		}
		if p.context.in(ContextObjectValue) {
			termination = append(termination, '}')
		}
		if p.context.in(ContextObjectKey) {
			termination = append(termination, ':')
		}

		if ok && ch == '#' {
			var comment []rune
			for ok && !inRunes(termination, ch) {
				comment = append(comment, ch)
				p.index++
				ch, ok = p.charAt(0)
			}
			p.log(fmt.Sprintf("Found line comment: %s, ignoring", string(comment)))
		} else if ok && ch == '/' {
			next, okNext := p.charAt(1)
			if okNext && next == '/' {
				comment := []rune{'/', '/'}
				p.index += 2
				c, cok := p.charAt(0)
				for cok && c != '\n' && c != '\r' {
					comment = append(comment, c)
					p.index++
					c, cok = p.charAt(0)
				}
				p.log(fmt.Sprintf("Found line comment: %s, ignoring", string(comment)))
			} else if okNext && next == '*' {
				comment := []rune{'/', '*'}
				p.index += 2
				for {
					c, cok := p.charAt(0)
					if !cok {
						p.log("Reached end-of-string while parsing block comment; unclosed block comment.")
						break
					}
					comment = append(comment, c)
					p.index++
					if endsWithRunes(comment, '*', '/') {
						break
					}
				}
				p.log(fmt.Sprintf("Found block comment: %s, ignoring", string(comment)))
			} else {
				// skip standalone '/' that is not part of a comment
				p.index++
			}
		}

		if p.context.empty {
			p.skipWhitespaces()
			nch, nok := p.charAt(0)
			if nok && (nch == '#' || nch == '/') {
				continue
			}
			return p.parseJSONSchema(nil, "$", true, recordTopLevelValue)
		}
		break
	}
	return ""
}

// ---- parenthesized / tuple ----

func (p *JSONParser) parseParenthesized() any {
	return p.parseParenthesizedSchema(nil, "$")
}

func (p *JSONParser) parseParenthesizedSchema(schema any, path string) any {
	explicitTuple := p.parenthesizedIsExplicitTuple()
	p.index++ // consume '('
	values := p.parseArraySchema(schema, path, ")")
	if explicitTuple || len(values) != 1 {
		return values
	}
	if len(values) > 0 {
		return values[0]
	}
	return []any{}
}

func (p *JSONParser) parenthesizedIsExplicitTuple() bool {
	i := p.index + 1
	n := len(p.jsonStr)
	nestedParens, squareBrackets, braces := 0, 0, 0
	var inQuote rune
	hasQuote := false
	backslashes := 0
	sawTopLevelContent := false

	for i < n {
		ch := p.jsonStr[i]
		if ch == '\\' {
			backslashes++
			i++
			continue
		}
		if hasQuote {
			if ch == inQuote && backslashes%2 == 0 {
				hasQuote = false
			}
			backslashes = 0
			i++
			continue
		}
		if isStringDelimiter(ch) && backslashes%2 == 0 {
			inQuote = ch
			hasQuote = true
			if nestedParens == 0 && squareBrackets == 0 && braces == 0 {
				sawTopLevelContent = true
			}
			backslashes = 0
			i++
			continue
		}
		backslashes = 0
		if !unicode.IsSpace(ch) && ch != ',' && ch != ')' && nestedParens == 0 && squareBrackets == 0 && braces == 0 {
			sawTopLevelContent = true
		}
		switch ch {
		case '(':
			nestedParens++
		case ')':
			if nestedParens == 0 && squareBrackets == 0 && braces == 0 {
				return !sawTopLevelContent
			}
			if nestedParens > 0 {
				nestedParens--
			}
		case '[':
			squareBrackets++
		case ']':
			if squareBrackets > 0 {
				squareBrackets--
			}
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case ',':
			if nestedParens == 0 && squareBrackets == 0 && braces == 0 {
				return true
			}
		}
		i++
	}
	return !sawTopLevelContent
}

func (p *JSONParser) topLevelParenthesizedCanStartValue() bool {
	i := p.index - 1
	for i >= 0 {
		ch := p.jsonStr[i]
		if ch == '\n' || ch == '\r' {
			break
		}
		if !unicode.IsSpace(ch) {
			return false
		}
		i--
	}
	idx := p.scrollWhitespaces(1)
	firstInner, ok := p.charAt(idx)
	if !ok {
		return false
	}
	innerText := strings.ToLower(string(p.jsonStr[p.index+idx:]))
	validFirst := isStringDelimiter(firstInner) ||
		firstInner == ')' || firstInner == '{' || firstInner == '[' || firstInner == '(' ||
		unicode.IsDigit(firstInner) || firstInner == '-' || firstInner == '.'
	if !validFirst &&
		!hasPrefixAny(innerText, "true", "null", "none") &&
		!strings.HasPrefix(innerText, "false") {
		return false
	}

	i2 := p.index + 1
	n := len(p.jsonStr)
	nestedParens, squareBrackets, braces := 0, 0, 0
	var inQuote rune
	hasQuote := false
	backslashes := 0
	for i2 < n {
		ch := p.jsonStr[i2]
		if ch == '\\' {
			backslashes++
			i2++
			continue
		}
		if hasQuote {
			if ch == inQuote && backslashes%2 == 0 {
				hasQuote = false
			}
			backslashes = 0
			i2++
			continue
		}
		if isStringDelimiter(ch) && backslashes%2 == 0 {
			inQuote = ch
			hasQuote = true
			backslashes = 0
			i2++
			continue
		}
		backslashes = 0
		switch ch {
		case '(':
			nestedParens++
		case ')':
			if nestedParens == 0 && squareBrackets == 0 && braces == 0 {
				i2++
				for i2 < n {
					trailer := p.jsonStr[i2]
					if trailer == '\n' || trailer == '\r' {
						return true
					}
					if !unicode.IsSpace(trailer) {
						return false
					}
					i2++
				}
				return true
			}
			nestedParens--
		case '[':
			squareBrackets++
		case ']':
			if squareBrackets > 0 {
				squareBrackets--
			}
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		}
		i2++
	}
	return true
}

// isTruthyJSON mirrors Python truthiness for parsed values: nil, false, "",
// empty containers and numeric 0 are falsy; everything else is truthy.
func isTruthyJSON(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case *OrderedMap:
		return x.Len() > 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	default:
		return true
	}
}

// ---- small rune helpers ----

func inRunes(set []rune, r rune) bool {
	for _, c := range set {
		if c == r {
			return true
		}
	}
	return false
}

func endsWithRunes(s []rune, tail ...rune) bool {
	if len(s) < len(tail) {
		return false
	}
	for i, t := range tail {
		if s[len(s)-len(tail)+i] != t {
			return false
		}
	}
	return true
}

func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
