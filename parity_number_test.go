// Code generated from ../json_repair-main/tests/test_parse_number.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestNumberParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `1`, want: `1`, skip: false, strict: false /* 1 */},
		{in: `1.2`, want: `1.2`, skip: false, strict: false /* 1.2 */},
		{in: `{"value": 82_461_110}`, want: `{"value": 82461110}`, skip: false, strict: false /* {"value": 82_461_110} */},
		{in: `{"value": 1_234.5_6}`, want: `{"value": 1234.56}`, skip: false, strict: false /* {"value": 1_234.5_6} */},
		{in: ` - { "test_key": ["test_value", "test_value2"] }`, want: `{"test_key": ["test_value", "test_value2"]}`, skip: false, strict: false /*  - { "test_key": ["test_value", "test_value2"] } */},
		{in: `{"key": 1/3}`, want: `{"key": "1/3"}`, skip: false, strict: false /* {"key": 13} */},
		{in: `{"key": .25}`, want: `{"key": 0.25}`, skip: false, strict: false /* {"key": .25} */},
		{in: `{"here": "now", "key": 1/3, "foo": "bar"}`, want: `{"here": "now", "key": "1/3", "foo": "bar"}`, skip: false, strict: false /* {"here": "now", "key": 13, "foo": "bar"} */},
		{in: `{"key": 12345/67890}`, want: `{"key": "12345/67890"}`, skip: false, strict: false /* {"key": 1234567890} */},
		{in: `[105,12`, want: `[105, 12]`, skip: false, strict: false /* [105,12 */},
		{in: `{"key", 105,12,`, want: `{"key": "105,12"}`, skip: false, strict: false /* {"key", 105,12, */},
		{in: `{"key": 1/3, "foo": "bar"}`, want: `{"key": "1/3", "foo": "bar"}`, skip: false, strict: false /* {"key": 13, "foo": "bar"} */},
		{in: `{"key": 10-20}`, want: `{"key": "10-20"}`, skip: false, strict: false /* {"key": 10-20} */},
		{in: `{"key": 1.1.1}`, want: `{"key": "1.1.1"}`, skip: false, strict: false /* {"key": 1.1.1} */},
		{in: `[- `, want: `[]`, skip: false, strict: false /* [-  */},
		{in: `{"key": 1. }`, want: `{"key": 1.0}`, skip: false, strict: false /* {"key": 1. } */},
		{in: `{"key": 1e10 }`, want: `{"key": 10000000000.0}`, skip: false, strict: false /* {"key": 1e10 } */},
		{in: `{"key": 1e }`, want: `{"key": 1}`, skip: false, strict: false /* {"key": 1e } */},
		{in: `{"key": 1notanumber }`, want: `{"key": "1notanumber"}`, skip: false, strict: false /* {"key": 1notanumber } */},
		{in: `{"rowId": 57eeeeb1-450b-482c-81b9-4be77e95dee2}`, want: `{"rowId": "57eeeeb1-450b-482c-81b9-4be77e95dee2"}`, skip: false, strict: false /* {"rowId": 57eeeeb1-450b-482c-81b9-4be77e95dee2} */},
		{in: `[1, 2notanumber]`, want: `[1, "2notanumber"]`, skip: false, strict: false /* [1, 2notanumber] */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
