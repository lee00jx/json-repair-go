package json_repair

import "testing"

// stream_stable parity. The Python suite has no dedicated stream_stable test,
// but the flag changes behavior on truncated (streaming) input, so this locks
// in the 1:1 semantics — verified against the Python oracle (repair_json with
// stream_stable True/False) on 2026-09-20.
//
// The single observable difference: when an unterminated string value ends in a
// dangling backslash (the start of an incomplete escape) at end-of-input,
// stream_stable=True drops that trailing backslash (it may gain a partner as the
// stream continues); stream_stable=False keeps it. All other sampled truncated
// inputs behave identically under both flags.
func TestStreamStableParity(t *testing.T) {
	// The stream_stable-specific case: dangling backslash at end of input.
	// stream_stable=False keeps the backslash; True strips it.
	dangling := `{"a": "hello ` + `\`
	assertEq(t, repairObj(t, dangling, false), map[string]any{"a": "hello \\"})
	assertEq(t, repairObj(t, dangling, true), map[string]any{"a": "hello "})

	// A completed escape (backslash+quote) is unaffected by the flag.
	escaped := `{"a": "x` + `\` + `"`
	assertEq(t, repairObj(t, escaped, false), map[string]any{"a": `x"`})
	assertEq(t, repairObj(t, escaped, true), map[string]any{"a": `x"`})

	// Common truncated inputs agree under both flags.
	agree := []struct {
		in   string
		want any
	}{
		{`{"a": "hello`, map[string]any{"a": "hello"}},
		{`{"a": 1,`, map[string]any{"a": int64(1)}},
		{`[1, 2, 3`, []any{int64(1), int64(2), int64(3)}},
		{`{"a": "val", "b": 2`, map[string]any{"a": "val", "b": int64(2)}},
		{`"unterminated string`, ""},
		{`{"a": 123`, map[string]any{"a": int64(123)}},
	}
	for _, c := range agree {
		assertEq(t, repairObj(t, c.in, false), c.want)
		assertEq(t, repairObj(t, c.in, true), c.want)
	}
}

func repairObj(t *testing.T, s string, streamStable bool) any {
	t.Helper()
	v, _, err := RepairJSON(s, Options{ReturnObjects: true, SkipJsonLoads: true, StreamStable: streamStable})
	if err != nil {
		t.Fatalf("repair %q (streamStable=%v): %v", s, streamStable, err)
	}
	return v
}
