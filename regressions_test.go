package json_repair

import (
	"reflect"
	"testing"
)

// TestBigIntConsistencyAcrossPaths locks the fix for the skip_json_loads
// inconsistency: an integer literal outside the int64 range must decode to the
// same Go type on the stdlib fast path and the repair (slow) path. Both now
// yield float64 (DECISIONS D-002). Before the fix the fast path returned
// float64 while the slow path returned a string.
func TestBigIntConsistencyAcrossPaths(t *testing.T) {
	in := `{"n": 18446744073709551616}` // 2^64, > MaxInt64
	fast := toPlainValue(rep(t, in, false)).(map[string]any)["n"]
	slow := toPlainValue(rep(t, in, true)).(map[string]any)["n"]
	if _, ok := fast.(float64); !ok {
		t.Fatalf("fast path: expected float64, got %T (%#v)", fast, fast)
	}
	if _, ok := slow.(float64); !ok {
		t.Fatalf("slow path: expected float64, got %T (%#v)", slow, slow)
	}
	if !reflect.DeepEqual(fast, slow) {
		t.Fatalf("fast and slow paths disagree: fast=%#v slow=%#v", fast, slow)
	}
	// The invalid-JSON form also forces the slow path.
	slowInv := toPlainValue(rep(t, in+",", false)).(map[string]any)["n"]
	if _, ok := slowInv.(float64); !ok {
		t.Fatalf("slow path (invalid json): expected float64, got %T (%#v)", slowInv, slowInv)
	}
}

// TestInRangeIntsStayInt64 guards against the big-int fix over-broadening:
// in-range integers and decimals must keep their D-002 types (int64/float64).
func TestInRangeIntsStayInt64(t *testing.T) {
	cases := []struct {
		in      string
		wantNum any
	}{
		{`{"v": 1}`, int64(1)},
		{`{"v": 42}`, int64(42)},
		{`{"v": 9223372036854775807}`, int64(9223372036854775807)}, // MaxInt64
		{`{"v": 1.5}`, 1.5},
		{`{"v": 1e10}`, 1e10},
	}
	for _, c := range cases {
		got := toPlainValue(rep(t, c.in, true)).(map[string]any)["v"]
		if !reflect.DeepEqual(got, c.wantNum) {
			t.Fatalf("input %q: got %#v, want %#v", c.in, got, c.wantNum)
		}
	}
}

// TestNegativeIndentNoPanic locks the fix for the Dump panic on a negative
// Indent. Python json.dumps treats a negative indent exactly like indent=0
// (newlines, no leading spaces), so Go must clamp to zero rather than panic in
// strings.Repeat.
func TestNegativeIndentNoPanic(t *testing.T) {
	outNeg, _, err := RepairJSONString(`{"a":1,"b":2}`, Options{Pretty: true, Indent: -2})
	if err != nil {
		t.Fatalf("RepairJSONString(indent=-2): %v", err)
	}
	out0, _, err := RepairJSONString(`{"a":1,"b":2}`, Options{Pretty: true, Indent: 0})
	if err != nil {
		t.Fatalf("RepairJSONString(indent=0): %v", err)
	}
	if outNeg != out0 {
		t.Fatalf("indent=-2 should equal indent=0:\n got  %q\n want %q", outNeg, out0)
	}
	// Negative indent must not produce leading spaces on nested levels either.
	outDeep, _, err := RepairJSONString(`{"a":{"b":1}}`, Options{Pretty: true, Indent: -5})
	if err != nil {
		t.Fatalf("RepairJSONString(nested, indent=-5): %v", err)
	}
	for _, line := range splitLines(outDeep) {
		if line != "" && line[0] == ' ' {
			t.Fatalf("negative indent produced leading spaces: %q", line)
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// TestPythonReprScalar locks the scalar-repr formatting used in "X is not of
// type 't'" error messages. The function is intentionally scalar-only (the
// full nested repr renderer is a known, deliberately-unbuilt gap); this pins
// the scalar cases to Python's repr: None / True / False / float / int.
func TestPythonReprScalar(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "None"},
		{true, "True"},
		{false, "False"},
		{float64(1.0), "1.0"},
		{float64(1.5), "1.5"},
		{float64(-0.5), "-0.5"},
		{int64(42), "42"},
		{int(-7), "-7"},
		{uint64(5), "5"},
		{"hello", "'hello'"},
		{"", "''"},
	}
	for _, c := range cases {
		if got := pythonReprScalar(c.in); got != c.want {
			t.Fatalf("pythonReprScalar(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}
