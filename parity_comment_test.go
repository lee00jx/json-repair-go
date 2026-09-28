// Code generated from ../json_repair-main/tests/test_parse_comment.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestCommentParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `{ "key": { "key2": "value2" // comment }, "key3": "value3" }`, want: `{"key": {"key2": "value2"}}`, skip: false, strict: false /* { "key": { "key2": "value2"  comment }, "key3" */},
		{in: "{ \"key\": { \"key2\": \"value2\" // comment\n}, \"key3\": \"value3\" }", want: `{"key": {"key2": "value2"}, "key3": "value3"}`, skip: false, strict: false /* { "key": { "key2": "value2"  comment\n}, "key3" */},
		{in: `{ "key": { "key2": "value2" # comment }, "key3": "value3" }`, want: `{"key": {"key2": "value2"}, "key3": "value3"}`, skip: false, strict: false /* { "key": { "key2": "value2" # comment }, "key3": */},
		{in: `{ "key": { "key2": "value2" /* comment */ }, "key3": "value3" }`, want: `{"key": {"key2": "value2"}, "key3": "value3"}`, skip: false, strict: false /* { "key": { "key2": "value2"  comment  }, "ke */},
		{in: `[ "value", /* comment */ "value2" ]`, want: `["value", "value2"]`, skip: false, strict: false /* [ "value",  comment  "value2" ] */},
		{in: `{ "key": "value" /* comment`, want: `{"key": "value"}`, skip: false, strict: false /* { "key": "value"  comment */},
		{in: "{\n// comment ]\n}", want: `{}`, skip: true, strict: false /* {\n comment ]\n} */},
		{in: `{/* comment ] */}`, want: `{}`, skip: true, strict: false /* { comment ] } */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
