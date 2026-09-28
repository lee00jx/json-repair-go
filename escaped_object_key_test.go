package json_repair

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Regression test for the "escaped object key" branch of
// classifyEmptyObjectRepair (parse_object.go): an object that parses empty
// whose body starts like {\"key\": ...} is normalized (\" -> ") and reparsed
// as an object. The expected values were captured from the real Python
// implementation (tools/diff_check corpus indices 5469/5473/5478/5502/5533/
// 5535/5536/5544) and are the 1:1 spec.
//
// History: the Go port originally checked for the 4-char pattern `\"":`
// (backslash, quote, colon, quote) instead of Python's 3-char `\":`, so the
// branch never fired and these inputs collapsed to empty objects.
func TestEscapedObjectKeyRepair(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: `{"a": {\"b\": 1}"}`, want: `{"a": {"b": 1}}`},
		{in: `{"a" "{\"b\": 1}"}`, want: `{"a": {"b": 1}}`},
		{in: `"a": "{\"b\": 1}"}`, want: `{"b": 1}`},
		{in: `{"a"}: "{\"b\": 1}"}`, want: `{"b": 1}`},
		{in: `{"a": %{\"b\": 1}"}`, want: `{"a": {"b": 1}}`},
		{in: `\"a": "{\"b\": 1}"}`, want: `{"b": 1}`},
		{in: `{"a": ^{\"b\": 1}"}`, want: `{"a": {"b": 1}}`},
		{in: `""a": "{\"b\": 1}"}`, want: `{"b": 1}`},
	}
	// The differential harness found these identical across all four
	// skip_json_loads x stream_stable combos; lock all four in.
	for _, skip := range []bool{false, true} {
		for _, ss := range []bool{false, true} {
			for _, c := range cases {
				t.Run("", func(t *testing.T) {
					got, _, err := RepairJSON(c.in, Options{SkipJsonLoads: skip, StreamStable: ss})
					if err != nil {
						t.Fatalf("RepairJSON(%q, skip=%v, ss=%v): %v", c.in, skip, ss, err)
					}
					var wantVal any
					if err := json.Unmarshal([]byte(c.want), &wantVal); err != nil {
						t.Fatalf("bad want JSON %q: %v", c.want, err)
					}
					if !reflect.DeepEqual(normNum(got), normNum(wantVal)) {
						t.Fatalf("input %q (skip=%v, ss=%v)\n got  %#v\n want %#v", c.in, skip, ss, got, wantVal)
					}
				})
			}
		}
	}
}
