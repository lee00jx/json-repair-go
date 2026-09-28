package json_repair

import (
	"strconv"
	"unicode"
)

// literalValues mirrors parse_string_helpers.parse_boolean_or_null.LITERAL_VALUES.
// "none" is the Python-specific unquoted null.
var literalValues = []struct {
	literal string
	value   any
}{
	{"true", true},
	{"false", false},
	{"null", nil},
	{"none", nil},
}

// parseBooleanOrNull returns whether an unquoted complete literal was found at
// the cursor and its value. A literal only counts if followed by a JSON
// separator or end-of-input; the cursor is restored on failure.
func (p *JSONParser) parseBooleanOrNull() (any, bool) {
	ch, ok := p.charAt(0)
	if !ok {
		return nil, false
	}
	startingIndex := p.index
	lower := unicode.ToLower(ch)
	for _, lv := range literalValues {
		if lower != rune(lv.literal[0]) {
			continue
		}
		p.index = startingIndex
		matched := true
		for _, expected := range lv.literal {
			c, ok2 := p.charAt(0)
			if !ok2 || unicode.ToLower(c) != expected {
				matched = false
				break
			}
			p.index++
		}
		if matched {
			next, okNext := p.charAt(0)
			if !okNext || unicode.IsSpace(next) || next == ',' || next == ')' || next == ']' || next == '}' {
				if lv.literal == "none" {
					p.log("Converted unquoted Python None literal to JSON null")
				}
				return lv.value, true
			}
		}
	}
	p.index = startingIndex
	return nil, false
}

// ---- string parsing state machine (port of Python parse_string.py) ----

const lowSmartQuoteSentinel = '\x00'

// stringParseState mirrors Python StringParseState.
type stringParseState struct {
	missingQuotes               bool
	doubledQuotes               bool
	lstringDelimiter            rune
	rstringDelimiter            []rune // stack; last is active, [0] is outer
	stringAcc                   []rune
	unmatchedDelimiter          bool
	pendingInlineContainer      bool
	inlineContainerStack        []rune
	objectValueHasNoFutureDelim bool
	lookaheadCache              map[string]cachedSkip
	objectValueUnmatchedBraces  int
	regexCharacterClassStart    *int
}

type cachedSkip struct {
	start    int
	match    int
	hasMatch bool
}

func newStringParseState() *stringParseState {
	return &stringParseState{
		lstringDelimiter: '"',
		rstringDelimiter: []rune{'"'},
		lookaheadCache:   map[string]cachedSkip{},
	}
}

func outerRStringDelim(state *stringParseState) rune { return state.rstringDelimiter[0] }
func activeRStringDelim(state *stringParseState) rune {
	return state.rstringDelimiter[len(state.rstringDelimiter)-1]
}

func inLowSmartQuoteSpan(state *stringParseState) bool {
	return activeRStringDelim(state) == lowSmartQuoteSentinel
}
func pushLowSmartQuoteSpan(state *stringParseState) {
	state.rstringDelimiter = append(state.rstringDelimiter, lowSmartQuoteSentinel)
}
func popLowSmartQuoteSpan(state *stringParseState) {
	state.rstringDelimiter = state.rstringDelimiter[:len(state.rstringDelimiter)-1]
}

func intPtr(v int) *int { return &v }

// isAlnum mirrors Python str.isalnum for the character set we handle.
func isAlnum(ch rune) bool { return unicode.IsLetter(ch) || unicode.IsDigit(ch) }

// parseString is the string parser. It ports the Python parse_string state
// machine: entry preparation (delimiters, doubled quotes, empty strings, code
// fences), the body scan (escapes, low smart quotes, inline containers, object
// value comma/brace classification, misplaced-quote handling), and finalization.
func (p *JSONParser) parseString() any {
	state, direct, hasDirect := p.prepareStringEntry()
	if hasDirect {
		return direct
	}
	ch, ok := p.scanStringBody(state)
	return p.finalizeStringResult(state, ch, ok)
}

// prepareStringEntry mirrors _prepare_string_entry. Returns (state, direct,
// hasDirect); when hasDirect is true, direct is the final result and state may
// be nil.
func (p *JSONParser) prepareStringEntry() (*stringParseState, any, bool) {
	char, ok := p.charAt(0)
	if !ok {
		return nil, "", true
	}
	if char == '#' || char == '/' {
		return nil, p.parseComment(false), true
	}

	for ok && !isStringDelimiter(char) && !isAlnum(char) {
		p.index++
		char, ok = p.charAt(0)
	}
	if !ok {
		return nil, "", true
	}

	if v, done := p.tryParseSimpleQuotedString(); done {
		return nil, v, true
	}

	state := newStringParseState()

	switch {
	case char == '\'':
		state.lstringDelimiter, state.rstringDelimiter = '\'', []rune{'\''}
	case char == '\u201c':
		state.lstringDelimiter, state.rstringDelimiter = '\u201c', []rune{'\u201d'}
	case isAlnum(char):
		if l := unicode.ToLower(char); (l == 't' || l == 'f' || l == 'n') && !p.context.currentIs(ContextObjectKey) {
			if v, found := p.parseBooleanOrNull(); found {
				return state, v, true
			}
		}
		p.log("While parsing a string, we found a literal instead of a quote")
		state.missingQuotes = true
	}

	if !state.missingQuotes {
		p.index++ // consume opening quote
	}
	if c, _ := p.charAt(0); c == '`' {
		if ret, ok2 := p.parseJSONLLMBlock(); ok2 {
			return state, ret, true
		}
		p.log("While parsing a string, we found code fences but they did not enclose valid JSON, continuing parsing the string")
	}

	if p.charAtIs(0, state.lstringDelimiter) {
		if (p.context.currentIs(ContextObjectKey) && p.charAtIs(1, ':')) ||
			(p.context.currentIs(ContextObjectValue) && (p.charAtIs(1, ',') || p.charAtIs(1, '}'))) ||
			(p.context.currentIs(ContextArray) && (p.charAtIs(1, ',') || p.charAtIs(1, ']'))) {
			p.index++
			return state, "", true
		}
		if p.charAtIs(1, state.lstringDelimiter) {
			p.log("While parsing a string, we found a doubled quote and then a quote again, ignoring it")
			if p.strict {
				p.fail("doubled quotes followed by another quote while parsing a string.")
				return state, "", true
			}
			return state, "", true
		}
		i := p.skipToCharacter([]rune{outerRStringDelim(state)}, 1)
		if p.charAtIs(i+1, outerRStringDelim(state)) {
			p.log("While parsing a string, we found a valid starting doubled quote")
			state.doubledQuotes = true
			p.index++
		} else {
			if p.charAtIs(i, outerRStringDelim(state)) {
				outerQuoteIdx := p.skipToCharacter([]rune{outerRStringDelim(state)}, i+1)
				afterOuterQuoteIdx := p.scrollWhitespaces(outerQuoteIdx + 1)
				afterOuterQuote, _ := p.charAt(afterOuterQuoteIdx)
				if p.context.currentIs(ContextObjectValue) &&
					!p.onlyWhitespaceUntil(i) &&
					p.charAtIs(outerQuoteIdx, outerRStringDelim(state)) &&
					(afterOuterQuote == ',' || afterOuterQuote == '}') {
					p.log("While parsing a string, we found a leading quote that starts a quoted span, keeping it")
					appendStringContent(state, []rune{state.lstringDelimiter})
					state.unmatchedDelimiter = true
					p.index++
					return state, nil, false
				}
			}
			i = p.scrollWhitespaces(1)
			nextC, _ := p.charAt(i)
			if isStringDelimiter(nextC) || nextC == '{' || nextC == '[' {
				p.log("While parsing a string, we found a doubled quote but also another quote afterwards, ignoring it")
				if p.strict {
					p.fail("doubled quotes followed by another quote while parsing a string")
					return state, "", true
				}
				p.index++
				return state, "", true
			}
			if nextC != ',' && nextC != ']' && nextC != '}' {
				p.log("While parsing a string, we found a doubled quote but it was a mistake, removing one quote")
				p.index++
			}
		}
	}

	return state, nil, false
}

// tryParseSimpleQuotedString mirrors _try_parse_simple_quoted_string.
func (p *JSONParser) tryParseSimpleQuotedString() (string, bool) {
	if c, _ := p.charAt(0); c != '"' {
		return "", false
	}
	start := p.index + 1
	n := len(p.jsonStr)
	end := start
	for end < n {
		char := p.jsonStr[end]
		if char == '"' {
			break
		}
		if char == '\\' || char == '\n' || char == '\r' {
			return "", false
		}
		end++
	}
	if end >= n {
		return "", false
	}
	value := string(p.jsonStr[start:end])

	nextIndex := end + 1
	for nextIndex < n && unicode.IsSpace(p.jsonStr[nextIndex]) {
		nextIndex++
	}
	nextChar, hasNext := func() (rune, bool) {
		if nextIndex < n {
			return p.jsonStr[nextIndex], true
		}
		return 0, false
	}()

	switch {
	case p.context.currentIs(ContextObjectKey):
		if !(hasNext && nextChar == ':') {
			return "", false
		}
	case p.context.currentIs(ContextObjectValue):
		if !(!hasNext || nextChar == ',' || nextChar == '}') {
			return "", false
		}
	case p.context.currentIs(ContextArray):
		if !(!hasNext || nextChar == ',' || nextChar == ']') {
			return "", false
		}
	default:
		if hasNext {
			return "", false
		}
	}
	p.index = end + 1
	return value, true
}

// parseJSONLLMBlock mirrors parse_json_llm_block.parse_json_llm_block.
func (p *JSONParser) parseJSONLLMBlock() (any, bool) {
	if hasPrefixRunes(p.jsonStr[p.index:], 7, []rune("```json")) {
		i := p.skipToCharacter([]rune{'`'}, 7)
		if hasPrefixRunes(p.jsonStr[p.index+i:], 3, []rune("```")) {
			p.index += 7
			return p.parseJSON(), true
		}
	}
	return nil, false
}

// onlyWhitespaceUntil mirrors _only_whitespace_until.
func (p *JSONParser) onlyWhitespaceUntil(end int) bool {
	for j := 1; j < end; j++ {
		c, ok := p.charAt(j)
		if ok && !unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

func appendStringContent(state *stringParseState, content []rune) {
	startIndex := len(state.stringAcc)
	state.stringAcc = append(state.stringAcc, content...)
	for offset, char := range content {
		if char == '{' {
			state.objectValueUnmatchedBraces++
		} else if char == '}' && state.objectValueUnmatchedBraces > 0 {
			state.objectValueUnmatchedBraces--
		} else if char == '[' {
			state.regexCharacterClassStart = intPtr(startIndex + offset + 1)
		} else if char == ']' {
			state.regexCharacterClassStart = nil
		}
	}
}

func rebuildUnmatchedBraces(state *stringParseState) {
	state.objectValueUnmatchedBraces = 0
	state.regexCharacterClassStart = nil
	for index, char := range state.stringAcc {
		if char == '{' {
			state.objectValueUnmatchedBraces++
		} else if char == '}' && state.objectValueUnmatchedBraces > 0 {
			state.objectValueUnmatchedBraces--
		} else if char == '[' {
			state.regexCharacterClassStart = intPtr(index + 1)
		} else if char == ']' {
			state.regexCharacterClassStart = nil
		}
	}
}

func (p *JSONParser) appendLiteralChar(state *stringParseState, char rune) (rune, bool) {
	appendStringContent(state, []rune{char})
	p.index++
	return p.charAt(0)
}

func (p *JSONParser) quoteBelongsToRegexCharClass(state *stringParseState) bool {
	start := state.regexCharacterClassStart
	if start == nil {
		return false
	}
	for _, char := range state.stringAcc[*start:] {
		if unicode.IsSpace(char) {
			return false
		}
	}
	closingIdx := p.skipToCharacter([]rune{']'}, 1)
	ch, ok := p.charAt(closingIdx)
	return ok && ch == ']'
}

func (p *JSONParser) cachedSkipToCharacter(state *stringParseState, targets []rune, idx int) int {
	key := string(targets)
	startIndex := p.index + idx
	if c, ok := state.lookaheadCache[key]; ok {
		if !c.hasMatch && startIndex >= c.start {
			return len(p.jsonStr) - p.index
		}
		if c.hasMatch && c.start <= startIndex && startIndex <= c.match {
			return c.match - p.index
		}
	}
	matchOffset := p.skipToCharacter(targets, idx)
	if _, ok := p.charAt(matchOffset); !ok {
		state.lookaheadCache[key] = cachedSkip{start: startIndex, hasMatch: false}
		return matchOffset
	}
	matchIndex := p.index + matchOffset
	if matchIndex == 0 || p.jsonStr[matchIndex-1] != '\\' {
		state.lookaheadCache[key] = cachedSkip{start: startIndex, match: matchIndex, hasMatch: true}
	}
	return matchOffset
}

// scanStringBody mirrors _scan_string_body. Returns the stopping character
// (and whether it exists).
func (p *JSONParser) scanStringBody(state *stringParseState) (rune, bool) {
	outer := outerRStringDelim(state)
	cachedSkip := func(targets []rune, idx int) int { return p.cachedSkipToCharacter(state, targets, idx) }

	ch, ok := p.charAt(0)
	for ok && (ch != outer || inLowSmartQuoteSpan(state)) {
		if state.missingQuotes {
			if p.context.currentIs(ContextObjectKey) && (ch == ':' || unicode.IsSpace(ch)) {
				p.log("While parsing a string missing the left delimiter in object key context, we found a :, stopping here")
				break
			}
			if p.context.currentIs(ContextArray) && (ch == ']' || ch == ',') {
				p.log("While parsing a string missing the left delimiter in array context, we found a ] or ,, stopping here")
				break
			}
		}
		if ch == '\u201e' && (len(state.stringAcc) == 0 || state.stringAcc[len(state.stringAcc)-1] != '\\') {
			pushLowSmartQuoteSpan(state)
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		if inLowSmartQuoteSpan(state) && ch == '\u201d' {
			popLowSmartQuoteSpan(state)
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		if (state.pendingInlineContainer ||
			(p.context.currentIs(ContextObjectValue) && ch == '{' && !p.charAtIs(-1, '\\') &&
				p.bareKeyIsFollowedByColon(p.scrollWhitespaces(1)))) &&
			(ch == '[' || ch == '{' || ch == '(') &&
			(len(state.stringAcc) == 0 || state.stringAcc[len(state.stringAcc)-1] != '\\') {
			containerEndIdx := skipInlineContainer(p, 0)
			if containerEndIdx >= 0 {
				p.log("While parsing a string in object value context, we found a balanced inline container that belongs to the string, keeping it")
				state.pendingInlineContainer = false
				state.inlineContainerStack = nil
				appendStringContent(state, p.jsonStr[p.index:p.index+containerEndIdx])
				p.index += containerEndIdx
				ch, ok = p.charAt(0)
				continue
			}
		}
		if !p.streamStable && p.context.currentIs(ContextObjectValue) && ch == ',' &&
			!state.pendingInlineContainer && len(state.inlineContainerStack) == 0 {
			classification := "string"
			if !state.objectValueHasNoFutureDelim {
				classification = classifyObjectValueComma(p, cachedSkip)
			}
			if classification == "member" {
				p.log("While parsing a string missing the right delimiter in object value context, we found a comma that starts the next object member. Stopping here")
				break
			}
			if classification == "string_no_future_delimiter" {
				state.objectValueHasNoFutureDelim = true
			}
			state.pendingInlineContainer = classification == "container"
			p.log("While parsing a string in object value context, we found a comma that belongs to the string, keeping it")
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		var keepInline bool
		state.pendingInlineContainer, keepInline = updateInlineContainerStack(ch, state.pendingInlineContainer, state.inlineContainerStack)
		if keepInline {
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		if !p.streamStable && p.context.currentIs(ContextObjectValue) && ch == '}' &&
			(len(state.stringAcc) == 0 || state.stringAcc[len(state.stringAcc)-1] != outer) {
			if state.objectValueUnmatchedBraces > 0 {
				ch, ok = p.appendLiteralChar(state, ch)
				continue
			}
			rstringDelimiterMissing := true
			p.skipWhitespaces()
			if p.charAtIs(1, '\\') {
				rstringDelimiterMissing = false
			}
			i := p.cachedSkipToCharacter(state, []rune{outer}, 1)
			nextC, nextOK := p.charAt(i)
			if nextOK {
				i++
				i = p.scrollWhitespaces(i)
				nextC, nextOK = p.charAt(i)
				if !nextOK || nextC == ',' || nextC == '}' {
					rstringDelimiterMissing = false
				} else {
					i = p.skipToCharacter([]rune{state.lstringDelimiter}, i)
					nextC, nextOK = p.charAt(i)
					if !nextOK {
						rstringDelimiterMissing = false
					} else {
						i = p.scrollWhitespaces(i + 1)
						nextC, nextOK = p.charAt(i)
						if nextOK && nextC != ':' {
							rstringDelimiterMissing = false
						}
					}
				}
			} else {
				i = p.skipToCharacter([]rune{':'}, 1)
				if _, ok2 := p.charAt(i); ok2 {
					break
				}
				i = p.scrollWhitespaces(1)
				j := p.skipToCharacter([]rune{'}'}, i)
				if j-i > 1 {
					rstringDelimiterMissing = false
				}
			}
			if rstringDelimiterMissing {
				p.log("While parsing a string missing the left delimiter in object value context, we found a , or } and we couldn't determine that a right delimiter was present. Stopping here")
				break
			}
		}
		if !p.streamStable && ch == ']' && p.context.in(ContextArray) &&
			(len(state.stringAcc) == 0 || state.stringAcc[len(state.stringAcc)-1] != outer) {
			i := p.skipToCharacter([]rune{outer}, 0)
			if _, ok2 := p.charAt(i); !ok2 {
				break
			}
		}
		if p.context.currentIs(ContextObjectValue) && ch == '}' {
			i := p.scrollWhitespaces(1)
			if p.charAtIs(i, '`') && p.charAtIs(i+1, '`') && p.charAtIs(i+2, '`') {
				if braceBeforeCodeFenceBelongsToString(p, state, i) {
					p.log("While parsing a string in object value context, we found a literal fenced snippet after }, keeping it in the string")
					ch, ok = p.appendLiteralChar(state, ch)
					continue
				}
				p.log("While parsing a string in object value context, we found a } that closes the object before code fences, stopping here")
				break
			}
			if _, ok2 := p.charAt(i); !ok2 {
				p.log("While parsing a string in object value context, we found a } that closes the object, stopping here")
				break
			}
		}
		appendStringContent(state, []rune{ch})
		p.index++
		ch, ok = p.charAt(0)
		if !ok {
			if p.streamStable && len(state.stringAcc) > 0 && state.stringAcc[len(state.stringAcc)-1] == '\\' {
				state.stringAcc = state.stringAcc[:len(state.stringAcc)-1]
				rebuildUnmatchedBraces(state)
			}
			break
		}
		if len(state.stringAcc) > 0 && state.stringAcc[len(state.stringAcc)-1] == '\\' {
			if handled, nch, nok := p.normalizeEscapeSequence(state, ch); handled {
				ch, ok = nch, nok
				continue
			}
		}
		if ch == ':' && !state.missingQuotes && p.context.currentIs(ContextObjectKey) {
			i := p.skipToCharacter([]rune{state.lstringDelimiter}, 1)
			_, nextOK := p.charAt(i)
			if nextOK {
				i++
				i = p.skipToCharacter([]rune{outer}, i)
				_, nextOK = p.charAt(i)
				if nextOK {
					i++
					i = p.scrollWhitespaces(i)
					c, _ := p.charAt(i)
					if c == ',' || c == '}' {
						p.log("While parsing a string missing the right delimiter in object key context, we found a " + string(c) + " stopping here")
						break
					}
				}
			} else {
				p.log("While parsing a string missing the right delimiter in object key context, we found a :, stopping here")
				break
			}
		}
		if inLowSmartQuoteSpan(state) && ch == '"' {
			popLowSmartQuoteSpan(state)
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		if ch == outer && p.context.currentIs(ContextObjectValue) && p.quoteBelongsToRegexCharClass(state) {
			p.log("While parsing a string, we found a bare quote inside a regex character class, keeping it")
			ch, ok = p.appendLiteralChar(state, ch)
			continue
		}
		if ch == outer && len(state.stringAcc) > 0 && state.stringAcc[len(state.stringAcc)-1] != '\\' {
			handled, shouldBreak, nch, nok := p.handleRightDelimiterCandidate(state, ch)
			if shouldBreak {
				break
			}
			if handled {
				ch, ok = nch, nok
				continue
			}
		}
	}
	return ch, ok
}

// normalizeEscapeSequence mirrors _normalize_escape_sequence. Returns (handled,
// nextChar, nextOK).
func (p *JSONParser) normalizeEscapeSequence(state *stringParseState, char rune) (bool, rune, bool) {
	p.log("Found a stray escape sequence, normalizing it")
	active := activeRStringDelim(state)
	if inLowSmartQuoteSpan(state) && char == '"' {
		state.stringAcc = append(state.stringAcc[:len(state.stringAcc)-1], char)
		rebuildUnmatchedBraces(state)
		popLowSmartQuoteSpan(state)
		p.index++
		rc, rok := p.charAt(0)
		return true, rc, rok
	}
	if char == '\\' {
		runStart := p.index - 1
		runEnd := p.index + 1
		for runEnd < len(p.jsonStr) && p.jsonStr[runEnd] == '\\' {
			runEnd++
		}
		runLength := runEnd - runStart
		nextChar, _ := p.charAt(runEnd - p.index)
		if runLength%2 == 0 && nextChar != active {
			half := runLength / 2
			replaced := make([]rune, 0, len(state.stringAcc)-1+half)
			replaced = append(replaced, state.stringAcc[:len(state.stringAcc)-1]...)
			for i := 0; i < half; i++ {
				replaced = append(replaced, '\\')
			}
			state.stringAcc = replaced
			rebuildUnmatchedBraces(state)
			p.index = runEnd
			rc, rok := p.charAt(0)
			return true, rc, rok
		}
	}
	if char == active || char == 't' || char == 'n' || char == 'r' || char == 'b' || char == '\\' {
		state.stringAcc = state.stringAcc[:len(state.stringAcc)-1]
		var replacement rune
		switch char {
		case 't':
			replacement = '\t'
		case 'n':
			replacement = '\n'
		case 'r':
			replacement = '\r'
		case 'b':
			replacement = '\b'
		default:
			replacement = char
		}
		state.stringAcc = append(state.stringAcc, replacement)
		rebuildUnmatchedBraces(state)
		p.index++
		nextChar, nextOK := p.charAt(0)
		for nextOK && len(state.stringAcc) > 0 && state.stringAcc[len(state.stringAcc)-1] == '\\' &&
			(nextChar == active || nextChar == '\\') {
			state.stringAcc = append(state.stringAcc[:len(state.stringAcc)-1], nextChar)
			rebuildUnmatchedBraces(state)
			p.index++
			nextChar, nextOK = p.charAt(0)
		}
		return true, nextChar, nextOK
	}
	if char == 'u' || char == 'x' {
		numChars := 4
		if char == 'x' {
			numChars = 2
		}
		lo := p.index + 1
		hi := lo + numChars
		if hi <= len(p.jsonStr) && hi == lo+numChars {
			nextChars := string(p.jsonStr[lo:hi])
			if len(nextChars) == numChars {
				allHex := true
				for _, c := range nextChars {
					if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
						allHex = false
						break
					}
				}
				if allHex {
					if code, err := strconv.ParseInt(nextChars, 16, 32); err == nil {
						p.log("Found a unicode escape sequence, normalizing it")
						state.stringAcc = append(state.stringAcc[:len(state.stringAcc)-1], rune(code))
						rebuildUnmatchedBraces(state)
						p.index = lo + numChars
						rc, rok := p.charAt(0)
						return true, rc, rok
					}
				}
			}
		}
		return false, char, true
	}
	if char == '\u201e' || (isStringDelimiter(char) && char != active) {
		p.log("Found a delimiter that was escaped but shouldn't be escaped, removing the escape")
		state.stringAcc = append(state.stringAcc[:len(state.stringAcc)-1], char)
		rebuildUnmatchedBraces(state)
		p.index++
		rc, rok := p.charAt(0)
		return true, rc, rok
	}
	return false, char, true
}

// handleRightDelimiterCandidate mirrors _handle_right_delimiter_candidate.
// Returns (handled, shouldBreak, nextChar, nextOK).
func (p *JSONParser) handleRightDelimiterCandidate(state *stringParseState, char rune) (bool, bool, rune, bool) {
	outer := outerRStringDelim(state)

	if state.doubledQuotes && p.charAtIs(1, outer) {
		p.log("While parsing a string, we found a doubled quote, ignoring it")
		p.index++
		return true, false, char, true
	}

	if state.missingQuotes && p.context.currentIs(ContextObjectValue) {
		i := 1
		nextC, okC := p.charAt(i)
		for okC && nextC != outer && nextC != state.lstringDelimiter {
			i++
			nextC, okC = p.charAt(i)
		}
		if okC {
			i++
			i = p.scrollWhitespaces(i)
			if p.charAtIs(i, ':') {
				p.index--
				nextChar, nextOK := p.charAt(0)
				p.log("In a string with missing quotes and object value context, I found a delimeter but it turns out it was the beginning on the next key. Stopping here.")
				return false, true, nextChar, nextOK
			}
		}
		return false, false, char, true
	}

	if state.unmatchedDelimiter {
		state.unmatchedDelimiter = false
		nextChar, nextOK := p.appendLiteralChar(state, char)
		return true, false, nextChar, nextOK
	}

	i := 1
	nextC, okC := p.charAt(i)
	checkCommaInObjectValue := true
	for okC && nextC != outer && nextC != state.lstringDelimiter {
		if checkCommaInObjectValue && unicode.IsLetter(nextC) {
			checkCommaInObjectValue = false
		}
		if (p.context.in(ContextObjectKey) && (nextC == ':' || nextC == '}')) ||
			(p.context.in(ContextObjectValue) && nextC == '}') ||
			(p.context.in(ContextArray) && (nextC == ']' || nextC == ',')) ||
			(checkCommaInObjectValue && p.context.currentIs(ContextObjectValue) && nextC == ',') {
			break
		}
		i++
		nextC, okC = p.charAt(i)
	}
	if nextC == ',' && p.context.currentIs(ContextObjectValue) {
		i++
		i = p.skipToCharacter([]rune{outer}, i)
		nextC, _ = p.charAt(i)
		i++
		i = p.scrollWhitespaces(i)
		nextC, _ = p.charAt(i)
		if nextC == '}' || nextC == ',' {
			p.log("While parsing a string, we found a misplaced quote that would have closed the string but has a different meaning here, ignoring it")
			nextChar, nextOK := p.appendLiteralChar(state, char)
			return true, false, nextChar, nextOK
		}
	} else if nextC == outer && (i-1 <= 0 || p.jsonStr[p.index+i-1] != '\\') {
		if p.onlyWhitespaceUntil(i) && !(p.context.currentIs(ContextObjectValue) && p.quotedObjectMemberFollows(i)) {
			return false, true, char, true
		}
		if p.context.currentIs(ContextObjectValue) {
			if p.quotedObjectMemberFollows(i) {
				p.log("While parsing a string, we found a misplaced quote that would have closed the string but has a different meaning here, ignoring it")
				nextChar, nextOK := p.appendLiteralChar(state, char)
				return true, false, nextChar, nextOK
			}
			i = p.skipToCharacter([]rune{outer}, i+1)
			i++
			nextC, okC2 := p.charAt(i)
			for okC2 && nextC != ':' {
				if nextC == ',' || nextC == ']' || nextC == '}' || (nextC == outer && (i-1 <= 0 || p.jsonStr[p.index+i-1] != '\\')) {
					break
				}
				i++
				nextC, okC2 = p.charAt(i)
			}
			if nextC != ':' {
				p.log("While parsing a string, we found a misplaced quote that would have closed the string but has a different meaning here, ignoring it")
				state.unmatchedDelimiter = !state.unmatchedDelimiter
				nextChar, nextOK := p.appendLiteralChar(state, char)
				return true, false, nextChar, nextOK
			}
		} else if p.context.currentIs(ContextArray) {
			evenDelimiters := nextC == outer
			for nextC == outer {
				i = p.skipToCharacter([]rune{outer, ']'}, i+1)
				nextC, _ = p.charAt(i)
				if nextC != outer {
					evenDelimiters = false
					break
				}
				i = p.skipToCharacter([]rune{outer, ']'}, i+1)
				nextC, _ = p.charAt(i)
			}
			if evenDelimiters {
				p.log("While parsing a string in Array context, we detected a quoted section that would have closed the string but has a different meaning here, ignoring it")
				state.unmatchedDelimiter = !state.unmatchedDelimiter
				nextChar, nextOK := p.appendLiteralChar(state, char)
				return true, false, nextChar, nextOK
			}
			return false, true, char, true
		} else if p.context.currentIs(ContextObjectKey) {
			p.log("While parsing a string in Object Key context, we detected a quoted section that would have closed the string but has a different meaning here, ignoring it")
			nextChar, nextOK := p.appendLiteralChar(state, char)
			return true, false, nextChar, nextOK
		}
	}

	return false, false, char, true
}

// finalizeStringResult mirrors _finalize_string_result.
func (p *JSONParser) finalizeStringResult(state *stringParseState, char rune, ok bool) string {
	outer := outerRStringDelim(state)
	if ok && state.missingQuotes && p.context.currentIs(ContextObjectKey) && unicode.IsSpace(char) {
		p.log("While parsing a string, handling an extreme corner case in which the LLM added a comment instead of valid string, invalidate the string and return an empty value")
		p.skipWhitespaces()
		if c, _ := p.charAt(0); c != ':' && c != ',' {
			return ""
		}
	}

	if !(ok && char == outer) {
		if !p.streamStable {
			p.log("While parsing a string, we missed the closing quote, ignoring")
			state.stringAcc = trimRightSpaceRunes(state.stringAcc)
		}
	} else {
		p.index++
	}

	if !p.streamStable && (state.missingQuotes || (len(state.stringAcc) > 0 && state.stringAcc[len(state.stringAcc)-1] == '\n')) {
		state.stringAcc = trimRightSpaceRunes(state.stringAcc)
	}

	return string(state.stringAcc)
}

// ---- inline container / object-member lookahead helpers ----

var inlineContainerClosing = map[rune]rune{'[': ']', '{': '}', '(': ')'}
var inlineContainerOpeners = []rune{'[', '{', '('}

func (p *JSONParser) bareKeyIsFollowedByColon(keyIdx int) bool {
	keyChar, ok := p.charAt(keyIdx)
	if !ok || !isAlnum(keyChar) && keyChar != '_' {
		return false
	}
	for {
		keyChar, ok = p.charAt(keyIdx)
		if !ok || !isAlnum(keyChar) && keyChar != '_' && keyChar != '-' {
			break
		}
		keyIdx++
	}
	keyIdx = p.scrollWhitespaces(keyIdx)
	return p.charAtIs(keyIdx, ':')
}

func (p *JSONParser) postFenceContainerStartsNextMember(containerEndIdx int) bool {
	afterIdx := p.scrollWhitespaces(containerEndIdx)
	after, _ := p.charAt(afterIdx)
	if after == '}' || !p.charAtValid(afterIdx) {
		return true
	}
	if after != ',' {
		return false
	}
	nextMemberIdx := p.scrollCommentPrefixedMemberStart(afterIdx + 1)
	c, ok := p.charAt(nextMemberIdx)
	return !ok || c == '}' || p.objectMemberStartsAt(nextMemberIdx)
}

func (p *JSONParser) charAtValid(idx int) bool {
	_, ok := p.charAt(idx)
	return ok
}

func (p *JSONParser) startsNestedInlineContainer(idx int) bool {
	openingDelimiter, _ := p.charAt(idx)
	prevIdx := idx - 1
	for prevIdx >= 0 {
		prevChar := p.jsonStr[prevIdx]
		if !unicode.IsSpace(prevChar) {
			if prevChar == '[' || prevChar == '{' || prevChar == '(' {
				return true
			}
			if prevChar != ',' && prevChar != ':' {
				return false
			}
			nextIdx := p.scrollWhitespaces(idx + 1)
			nextChar, okNext := p.charAt(nextIdx)
			if openingDelimiter == '[' || openingDelimiter == '(' {
				if okNext {
					return nextChar == ']' || nextChar == ')' || isStringDelimiter(nextChar) ||
						nextChar == '-' || (nextChar == '[' || nextChar == '{' || nextChar == '(') ||
						nextChar == 't' || nextChar == 'f' || nextChar == 'n' || unicode.IsDigit(nextChar)
				}
				return false
			}
			if openingDelimiter != '{' {
				return false
			}
			if okNext && (nextChar == '}' || isStringDelimiter(nextChar)) {
				return true
			}
			return prevChar == ':' && p.bareKeyIsFollowedByColon(nextIdx)
		}
		prevIdx--
	}
	return true
}

// skipInlineContainer mirrors _skip_inline_container. Returns the index just
// past the matching close (offset from p.index), or -1 if unbalanced/absent.
func skipInlineContainer(p *JSONParser, idx int) int {
	openingDelimiter, _ := p.charAt(idx)
	if _, isOpener := inlineContainerClosing[openingDelimiter]; !isOpener {
		return idx
	}
	stack := []rune{inlineContainerClosing[openingDelimiter]}
	i := idx + 1
	for len(stack) > 0 {
		char, ok := p.charAt(i)
		if !ok {
			return -1
		}
		if isStringDelimiter(char) {
			endDelim := matchingStringDelimiter(char)
			i = p.skipToCharacter([]rune{endDelim}, i+1)
			if !p.charAtIs(i, endDelim) {
				return -1
			}
		} else if (char == '[' || char == '{' || char == '(') && p.startsNestedInlineContainer(i) {
			stack = append(stack, inlineContainerClosing[char])
		} else if char == stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1
			}
		}
		i++
	}
	return -1
}

func (p *JSONParser) scrollCommentPrefixedMemberStart(idx int) int {
	idx = p.scrollWhitespaces(idx)
	for {
		char, ok := p.charAt(idx)
		if !ok {
			return idx
		}
		if char == '#' {
			for {
				char, ok = p.charAt(idx)
				if !ok || char == '\n' || char == '\r' {
					break
				}
				idx++
			}
			idx = p.scrollWhitespaces(idx)
			continue
		}
		if char == '/' {
			nextChar, _ := p.charAt(idx + 1)
			if nextChar == '/' {
				idx += 2
				for {
					char, ok = p.charAt(idx)
					if !ok || char == '\n' || char == '\r' {
						break
					}
					idx++
				}
				idx = p.scrollWhitespaces(idx)
				continue
			}
			if nextChar == '*' {
				idx += 2
				for {
					char, ok = p.charAt(idx)
					if !ok {
						return idx
					}
					if char == '*' && p.charAtIs(idx+1, '/') {
						idx += 2
						break
					}
					idx++
				}
				idx = p.scrollWhitespaces(idx)
				continue
			}
		}
		return idx
	}
}

func (p *JSONParser) quotedObjectMemberFollows(quoteIdx int) bool {
	commaIdx := p.scrollWhitespaces(quoteIdx + 1)
	if !p.charAtIs(commaIdx, ',') {
		return false
	}
	nextMemberIdx := p.scrollCommentPrefixedMemberStart(commaIdx + 1)
	return p.objectMemberStartsAt(nextMemberIdx)
}

func (p *JSONParser) objectMemberStartsAt(nextMemberIdx int) bool {
	if c, _ := p.charAt(nextMemberIdx); c == '}' || !p.charAtValid(nextMemberIdx) {
		return false
	}
	nextMember, _ := p.charAt(nextMemberIdx)
	if isStringDelimiter(nextMember) {
		keyEndDelimiter := matchingStringDelimiter(nextMember)
		keyEndIdx := p.skipToCharacter([]rune{keyEndDelimiter}, nextMemberIdx+1)
		if !p.charAtIs(keyEndIdx, keyEndDelimiter) {
			return false
		}
		afterKeyIdx := p.scrollWhitespaces(keyEndIdx + 1)
		return p.charAtIs(afterKeyIdx, ':')
	}
	if isAlnum(nextMember) || nextMember == '_' {
		return p.bareKeyIsFollowedByColon(nextMemberIdx)
	}
	return false
}

// braceBeforeCodeFenceBelongsToString mirrors _brace_before_code_fence_belongs_to_string.
func braceBeforeCodeFenceBelongsToString(p *JSONParser, state *stringParseState, fenceIdx int) bool {
	quoteSearchIdx := fenceIdx + 3
	nextContentIdx := p.scrollCommentPrefixedMemberStart(quoteSearchIdx)
	keepPostFenceContainer := false
	c, _ := p.charAt(nextContentIdx)
	if c == '[' || c == '{' || c == '(' {
		containerEndIdx := skipInlineContainer(p, nextContentIdx)
		if containerEndIdx >= 0 {
			if p.postFenceContainerStartsNextMember(containerEndIdx) {
				return false
			}
			keepPostFenceContainer = true
			quoteSearchIdx = containerEndIdx
		}
	}
	outer := outerRStringDelim(state)
	quoteIdx := p.skipToCharacter([]rune{outer}, quoteSearchIdx)
	for p.charAtIs(quoteIdx, outer) {
		afterQuoteIdx := p.scrollWhitespaces(quoteIdx + 1)
		afterQuote, okQ := p.charAt(afterQuoteIdx)
		if !okQ || afterQuote == ',' || afterQuote == '}' || afterQuote == ']' {
			if keepPostFenceContainer {
				state.pendingInlineContainer = true
			}
			return true
		}
		quoteIdx = p.skipToCharacter([]rune{outer}, quoteIdx+1)
	}
	return false
}

// ---- object value comma classification (port of object_value_context.py) ----

func bareMemberHasRecoverableValue(p *JSONParser, valueIdx int, skipTo func([]rune, int) int) bool {
	valueStartIdx := p.scrollWhitespaces(valueIdx)
	valueStart, _ := p.charAt(valueStartIdx)
	if isStringDelimiter(valueStart) || valueStart == '{' || valueStart == '[' || valueStart == '-' {
		return true
	}
	if unicode.IsDigit(valueStart) {
		return true
	}
	for _, literal := range []string{"true", "false", "null"} {
		all := true
		for offset, char := range literal {
			c, ok := p.charAt(valueStartIdx + offset)
			if !ok || c != char {
				all = false
				break
			}
		}
		if all {
			valueEnd, okEnd := p.charAt(valueStartIdx + len(literal))
			if !okEnd || unicode.IsSpace(valueEnd) || valueEnd == ',' || valueEnd == '}' || valueEnd == ']' {
				return true
			}
		}
	}
	valueEndIdx := skipTo(append(append([]rune{}, StringDelimiters...), '}'), valueStartIdx)
	return p.charAtIs(valueEndIdx, '}')
}

func classifyObjectValueComma(p *JSONParser, cachedSkip func([]rune, int) int) string {
	nextIdx := p.scrollWhitespaces(1)
	nextC, _ := p.charAt(nextIdx)
	if nextC == '}' || !p.charAtValid(nextIdx) {
		return "member"
	}
	if isStringDelimiter(nextC) {
		keyEndIdx := p.skipToCharacter([]rune{nextC}, nextIdx+1)
		if !p.charAtValid(keyEndIdx) {
			return "string"
		}
		keyEndIdx = p.scrollWhitespaces(keyEndIdx + 1)
		if p.charAtIs(keyEndIdx, ':') {
			return "member"
		}
		return "string"
	}
	if nextC == '`' {
		bareKeyIdx := nextIdx + 1
		for {
			keyChar, ok := p.charAt(bareKeyIdx)
			if !ok || !isAlnum(keyChar) && keyChar != '_' && keyChar != '-' {
				break
			}
			bareKeyIdx++
		}
		bareKeyIdx = p.scrollWhitespaces(bareKeyIdx)
		if p.charAtIs(bareKeyIdx, ':') {
			return "member"
		}
		return "string"
	}
	if isAlnum(nextC) || nextC == '_' {
		bareKeyIdx := nextIdx
		for {
			keyChar, ok := p.charAt(bareKeyIdx)
			if !ok || !isAlnum(keyChar) && keyChar != '_' && keyChar != '-' {
				break
			}
			bareKeyIdx++
		}
		bareKeyIdx = p.scrollWhitespaces(bareKeyIdx)
		if p.charAtIs(bareKeyIdx, ':') && bareMemberHasRecoverableValue(p, bareKeyIdx+1, cachedSkip) {
			return "member"
		}
	}
	if nextC == '{' || nextC == '[' {
		return "container"
	}
	nextSpecialIdx := cachedSkip(append(append(append([]rune{}, StringDelimiters...), '{'), '['), nextIdx)
	nextSpecial, okSpecial := p.charAt(nextSpecialIdx)
	if !okSpecial {
		return "string_no_future_delimiter"
	}
	if nextSpecial == '{' || nextSpecial == '[' {
		return "string"
	}
	keyEndIdx := cachedSkip([]rune{nextSpecial}, nextSpecialIdx+1)
	if !p.charAtValid(keyEndIdx) {
		return "string"
	}
	keyEndIdx = p.scrollWhitespaces(keyEndIdx + 1)
	if p.charAtIs(keyEndIdx, ':') {
		return "member"
	}
	return "string"
}

func updateInlineContainerStack(char rune, pending bool, stack []rune) (bool, bool) {
	if char == '{' || char == '[' {
		if pending {
			stack = append(stack, char)
			return false, false
		}
		if len(stack) > 0 {
			stack = append(stack, char)
		}
	}
	if len(stack) > 0 &&
		((char == '}' && stack[len(stack)-1] == '{') || (char == ']' && stack[len(stack)-1] == '[')) {
		stack = stack[:len(stack)-1]
		return pending, true
	}
	return pending, false
}

func hasPrefixRunes(s []rune, n int, target []rune) bool {
	if n != len(target) || len(s) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if s[i] != target[i] {
			return false
		}
	}
	return true
}

func trimRightSpaceRunes(s []rune) []rune {
	i := len(s)
	for i > 0 && unicode.IsSpace(s[i-1]) {
		i--
	}
	return s[:i]
}
