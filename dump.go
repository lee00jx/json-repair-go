package json_repair

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// DumpOptions controls serialization, mirroring the subset of json.dumps
// arguments used by repair_json / the CLI.
type DumpOptions struct {
	// EnsureASCII, when true, escapes non-ASCII characters as \uXXXX (Python
	// json.dumps default is ensure_ascii=True).
	EnsureASCII bool
	// Pretty selects pretty-printing (Python json.dumps indent != None). When
	// true, the item separator becomes "," + newline and each nested level is
	// indented by Indent spaces. Pretty with Indent=0 matches Python
	// indent=0 (newlines but no leading spaces).
	Pretty bool
	// Indent is the number of spaces per level, meaningful only when Pretty.
	Indent int
}

// Dump serializes a repaired JSON value to a string, matching Python
// json.dumps for the value types produced by the repairer (*OrderedMap,
// []any, string, bool, int64, uint64, float64, nil).
//
// Key order (DECISIONS D-006, closed): repairer output objects are
// *OrderedMap and are emitted in Python insertion order. Plain
// map[string]any values (only possible from user-constructed input, since the
// repairer never produces them) carry no order and are emitted with sorted
// keys as the deterministic convention.
func Dump(v any, opts DumpOptions) (string, error) {
	var sb strings.Builder
	if err := dumpValue(&sb, v, opts, 0); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func dumpValue(sb *strings.Builder, v any, opts DumpOptions, depth int) error {
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		if x {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case string:
		dumpString(sb, x, opts.EnsureASCII)
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case int:
		sb.WriteString(strconv.FormatInt(int64(x), 10))
	case uint64:
		sb.WriteString(strconv.FormatUint(x, 10))
	case float64:
		sb.WriteString(pythonFloatString(x))
	case *OrderedMap:
		if err := dumpOrderedObject(sb, x, opts, depth); err != nil {
			return err
		}
	case map[string]any:
		if err := dumpObject(sb, x, opts, depth); err != nil {
			return err
		}
	case []any:
		if err := dumpArray(sb, x, opts, depth); err != nil {
			return err
		}
	default:
		return fmt.Errorf("json_repair: cannot serialize value of type %T", v)
	}
	return nil
}

// dumpOrderedObject emits an OrderedMap in Python insertion order (D-006).
func dumpOrderedObject(sb *strings.Builder, m *OrderedMap, opts DumpOptions, depth int) error {
	if m.Len() == 0 {
		sb.WriteString("{}")
		return nil
	}
	sb.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			dumpComma(sb, opts)
		}
		if opts.Pretty {
			dumpIndent(sb, opts, depth+1)
		}
		dumpString(sb, k, opts.EnsureASCII)
		sb.WriteString(": ")
		if err := dumpValue(sb, m.values[k], opts, depth+1); err != nil {
			return err
		}
	}
	if opts.Pretty {
		dumpIndent(sb, opts, depth)
	}
	sb.WriteByte('}')
	return nil
}

// dumpObject emits a plain (orderless) map with sorted keys — the
// deterministic convention for input that carries no order (D-006).
func dumpObject(sb *strings.Builder, m map[string]any, opts DumpOptions, depth int) error {
	if len(m) == 0 {
		sb.WriteString("{}")
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			dumpComma(sb, opts)
		}
		if opts.Pretty {
			dumpIndent(sb, opts, depth+1)
		}
		dumpString(sb, k, opts.EnsureASCII)
		sb.WriteString(": ")
		if err := dumpValue(sb, m[k], opts, depth+1); err != nil {
			return err
		}
	}
	if opts.Pretty {
		dumpIndent(sb, opts, depth)
	}
	sb.WriteByte('}')
	return nil
}

func dumpArray(sb *strings.Builder, a []any, opts DumpOptions, depth int) error {
	if len(a) == 0 {
		sb.WriteString("[]")
		return nil
	}
	sb.WriteByte('[')
	for i, el := range a {
		if i > 0 {
			dumpComma(sb, opts)
		}
		if opts.Pretty {
			dumpIndent(sb, opts, depth+1)
		}
		if err := dumpValue(sb, el, opts, depth+1); err != nil {
			return err
		}
	}
	if opts.Pretty {
		dumpIndent(sb, opts, depth)
	}
	sb.WriteByte(']')
	return nil
}

// dumpComma writes the separator between elements: ", " compact, "," pretty
// (the newline/indent follows in pretty mode).
func dumpComma(sb *strings.Builder, opts DumpOptions) {
	if opts.Pretty {
		sb.WriteByte(',')
	} else {
		sb.WriteString(", ")
	}
}

func dumpIndent(sb *strings.Builder, opts DumpOptions, depth int) {
	sb.WriteByte('\n')
	// A negative Indent (e.g. RepairJSONString with Indent<0) used to panic in
	// strings.Repeat. Python json.dumps treats a negative indent exactly like
	// indent=0 (newlines, no leading spaces), so clamp to zero instead.
	n := opts.Indent * depth
	if n < 0 {
		n = 0
	}
	sb.WriteString(strings.Repeat(" ", n))
}

// dumpString writes a JSON string with Python json.dumps escaping.
func dumpString(sb *strings.Builder, s string, ensureASCII bool) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case '\r':
			sb.WriteString(`\r`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			needEscape := r < 0x20
			if ensureASCII && r > 0x7e {
				needEscape = true
			}
			if needEscape {
				sb.WriteString(pythonEscape(r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}

// pythonEscape mirrors Python's \u escaping for a single rune.
func pythonEscape(r rune) string {
	if r < 0x10000 {
		return "\\u" + hexLower(uint32(r))
	}
	// Surrogate pair for astral characters (Python json.dumps emits \ud800\udc00).
	c := uint32(r) - 0x10000
	hi := 0xD800 + (c >> 10)
	lo := 0xDC00 + (c & 0x3FF)
	return "\\u" + hexLower(hi) + "\\u" + hexLower(lo)
}

func hexLower(v uint32) string {
	const digits = "0123456789abcdef"
	if v < 0x1000 {
		if v < 0x100 {
			if v < 0x10 {
				return "000" + string(digits[v])
			}
			return "00" + string(digits[v>>4]) + string(digits[v&0xF])
		}
		return "0" + string(digits[v>>8]) + string(digits[(v>>4)&0xF]) + string(digits[v&0xF])
	}
	return string(digits[v>>12]) + string(digits[(v>>8)&0xF]) + string(digits[(v>>4)&0xF]) + string(digits[v&0xF])
}

// pythonFloatString formats a float64 exactly like Python's str()/repr() for
// finite floats: the shortest round-trip digits, in fixed form by default and
// scientific form only when the base-10 exponent E satisfies E < -4 or
// E >= 16 (CPython float_repr rule).
func pythonFloatString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	// Shortest round-trip scientific form: "d.dddde±xx" or "de±xx".
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expStr := splitAtE(e)
	E, _ := strconv.Atoi(expStr)
	digits := strings.ReplaceAll(mant, ".", "")
	// Sign handling.
	neg := strings.HasPrefix(e, "-")
	if neg {
		digits = strings.TrimPrefix(digits, "-")
	}

	sci := E < -4 || E >= 16
	var out string
	if sci {
		out = digits[0:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		out += "e"
		if E < 0 {
			out += "-"
			E = -E
		} else {
			out += "+"
		}
		es := strconv.Itoa(E)
		if len(es) < 2 {
			es = "0" + es
		}
		out += es
	} else {
		p := E + 1 // number of digits before the decimal point
		switch {
		case p <= 0:
			out = "0." + strings.Repeat("0", -p) + digits
		case p >= len(digits):
			out = digits + strings.Repeat("0", p-len(digits)) + ".0"
		default:
			out = digits[:p] + "." + digits[p:]
		}
	}
	if neg {
		out = "-" + out
	}
	return out
}

func splitAtE(s string) (string, string) {
	i := strings.IndexByte(s, 'e')
	return s[:i], s[i+1:]
}
