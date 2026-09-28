// Code generated from ../json_repair-main/tests/test_parse_array.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestArrayParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `[]`, want: `[]`, skip: false, strict: false /* [] */},
		{in: `[1, 2, 3, 4]`, want: `[1, 2, 3, 4]`, skip: false, strict: false /* [1, 2, 3, 4] */},
		{in: `[`, want: `[]`, skip: false, strict: false /* [ */},
		{in: "[[1\n\n]", want: `[[1]]`, skip: false, strict: false /* [[1\n\n] */},
		{in: `[{]`, want: `[]`, skip: false, strict: false /* [{] */},
		{in: `["`, want: `[]`, skip: false, strict: false /* [" */},
		{in: `[1, 2, 3,`, want: `[1, 2, 3]`, skip: false, strict: false /* [1, 2, 3, */},
		{in: `[1, 2, 3, ...]`, want: `[1, 2, 3]`, skip: false, strict: false /* [1, 2, 3, ...] */},
		{in: `[1, 2, ... , 3]`, want: `[1, 2, 3]`, skip: false, strict: false /* [1, 2, ... , 3] */},
		{in: `[1, 2, '...', 3]`, want: `[1, 2, "...", 3]`, skip: false, strict: false /* [1, 2, '...', 3] */},
		{in: `[true, false, null, ...]`, want: `[true, false, null]`, skip: false, strict: false /* [true, false, null, ...] */},
		{in: `["a" "b" "c" 1`, want: `["a", "b", "c", 1]`, skip: false, strict: false /* ["a" "b" "c" 1 */},
		{in: `{"employees":["John", "Anna",`, want: `{"employees": ["John", "Anna"]}`, skip: false, strict: false /* {"employees":["John", "Anna", */},
		{in: `{"employees":["John", "Anna", "Peter`, want: `{"employees": ["John", "Anna", "Peter"]}`, skip: false, strict: false /* {"employees":["John", "Anna", "Peter */},
		{in: `{"key1": {"key2": [1, 2, 3`, want: `{"key1": {"key2": [1, 2, 3]}}`, skip: false, strict: false /* {"key1": {"key2": [1, 2, 3 */},
		{in: `{"key": ["value]}`, want: `{"key": ["value"]}`, skip: false, strict: false /* {"key": ["value]} */},
		{in: `["lorem "ipsum" sic"]`, want: `["lorem \"ipsum\" sic"]`, skip: false, strict: false /* ["lorem "ipsum" sic"] */},
		{in: `{"key1": ["value1", "value2"}, "key2": ["value3", "value4"]}`, want: `{"key1": ["value1", "value2"], "key2": ["value3", "value4"]}`, skip: false, strict: false /* {"key1": ["value1", "value2"}, "key2": ["value3" */},
		{in: `{"headers": ["A", "B", "C"], "rows": [["r1a", "r1b", "r1c"], ["r2a", "r2b", "r2c"], "r3a", "r3b", "r3c"], ["r4a", "r4b", "r4c"], ["r5a", "r5b", "r5c"]]}`, want: `{"headers": ["A", "B", "C"], "rows": [["r1a", "r1b", "r1c"], ["r2a", "r2b", "r2c"], ["r3a", "r3b", "r3c"], ["r4a", "r4b", "r4c"], ["r5a", "r5b", "r5c"]]}`, skip: false, strict: false /* {"headers": ["A", "B", "C"], "rows": [["r1a", "r */},
		{in: `{"key": ["value" "value1" "value2"]}`, want: `{"key": ["value", "value1", "value2"]}`, skip: false, strict: false /* {"key": ["value" "value1" "value2"]} */},
		{in: `{"key": ["lorem "ipsum" dolor "sit" amet, "consectetur" ", "lorem "ipsum" dolor", "lorem"]}`, want: `{"key": ["lorem \"ipsum\" dolor \"sit\" amet, \"consectetur\" ", "lorem \"ipsum\" dolor", "lorem"]}`, skip: false, strict: false /* {"key": ["lorem "ipsum" dolor "sit" amet, "conse */},
		{in: `{"k"e"y": "value"}`, want: `{"k\"e\"y": "value"}`, skip: false, strict: false /* {"k"e"y": "value"} */},
		{in: `["key":"value"}]`, want: `[{"key": "value"}]`, skip: false, strict: false /* ["key":"value"}] */},
		{in: `["key":"value"]`, want: `[{"key": "value"}]`, skip: false, strict: false /* ["key":"value"] */},
		{in: `[ "key":"value"]`, want: `[{"key": "value"}]`, skip: false, strict: false /* [ "key":"value"] */},
		{in: `[{"key": "value", "key`, want: `[{"key": "value"}, ["key"]]`, skip: false, strict: false /* [{"key": "value", "key */},
		{in: `{'key1', 'key2'}`, want: `["key1", "key2"]`, skip: false, strict: false /* {'key1', 'key2'} */},
		{in: `{"outer": ["a", "b", "next": "value"}`, want: `{"outer": ["a", "b"], "next": "value"}`, skip: false, strict: true /* {"outer": ["a", "b", "next": "value"} */},
		{in: `("a", "b", "c")`, want: `["a", "b", "c"]`, skip: false, strict: false /* ("a", "b", "c") */},
		{in: `((1, 2), (3, 4))`, want: `[[1, 2], [3, 4]]`, skip: false, strict: false /* ((1, 2), (3, 4)) */},
		{in: `{"coords": (1, 2), "ok": true}`, want: `{"coords": [1, 2], "ok": true}`, skip: false, strict: false /* {"coords": (1, 2), "ok": true} */},
		{in: `{"empty": ()}`, want: `{"empty": []}`, skip: false, strict: false /* {"empty": ()} */},
		{in: `(true, false, null)`, want: `[true, false, null]`, skip: true, strict: false /* (true, false, null) */},
		{in: `(True, False, None)`, want: `[true, false, null]`, skip: true, strict: false /* (True, False, None) */},
		{in: `{"coords": (True, None)}`, want: `{"coords": [true, null]}`, skip: true, strict: false /* {"coords": (True, None)} */},
		{in: `(1)`, want: `1`, skip: false, strict: false /* (1) */},
		{in: `{"scalar_group": (1)}`, want: `{"scalar_group": 1}`, skip: false, strict: false /* {"scalar_group": (1)} */},
		{in: `{"string_group": ("x")}`, want: `{"string_group": "x"}`, skip: false, strict: false /* {"string_group": ("x")} */},
		{in: `[1, 2)`, want: `[1, 2]`, skip: false, strict: false /* [1, 2) */},
		{in: `["value1" value2", "value3"]`, want: `["value1", "value2", "value3"]`, skip: false, strict: false /* ["value1" value2", "value3"] */},
		{in: `{"bad_one":["Lorem Ipsum", "consectetur" comment" ], "good_one":[ "elit", "sed", "tempor"]}`, want: `{"bad_one": ["Lorem Ipsum", "consectetur", "comment"], "good_one": ["elit", "sed", "tempor"]}`, skip: false, strict: false /* {"bad_one":["Lorem Ipsum", "consectetur" comment */},
		{in: `{"bad_one": ["Lorem Ipsum","consectetur" comment],"good_one": ["elit","sed","tempor"]}`, want: `{"bad_one": ["Lorem Ipsum", "consectetur", "comment"], "good_one": ["elit", "sed", "tempor"]}`, skip: false, strict: false /* {"bad_one": ["Lorem Ipsum","consectetur" comment */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
