// Code generated from ../json_repair-main/tests/test_parse_object.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestObjectParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `{}`, want: `{}`, skip: false, strict: false /* {} */},
		{in: `{ "key": "value", "key2": 1, "key3": True }`, want: `{"key": "value", "key2": 1, "key3": true}`, skip: false, strict: false /* { "key": "value", "key2": 1, "key3": True } */},
		{in: `{`, want: `{}`, skip: false, strict: false /* { */},
		{in: `{ "key": value, "key2": 1 "key3": null }`, want: `{"key": "value", "key2": 1, "key3": null}`, skip: false, strict: false /* { "key": value, "key2": 1 "key3": null } */},
		{in: `   {  }   `, want: `{}`, skip: false, strict: false /*    {  }    */},
		{in: `{"`, want: `{}`, skip: false, strict: false /* {" */},
		{in: `{foo: [}`, want: `{"foo": []}`, skip: false, strict: false /* {foo: [} */},
		{in: `{"": "value"`, want: `{"": "value"}`, skip: false, strict: false /* {"": "value" */},
		{in: `{"key": "v"alue"}`, want: `{"key": "v\"alue\""}`, skip: false, strict: false /* {"key": "v"alue"} */},
		{in: `{"value_1": true, COMMENT "value_2": "data"}`, want: `{"value_1": true, "value_2": "data"}`, skip: false, strict: false /* {"value_1": true, COMMENT "value_2": "data"} */},
		{in: `{"value_1": true, SHOULD_NOT_EXIST "value_2": "data" AAAA }`, want: `{"value_1": true, "value_2": "data"}`, skip: false, strict: false /* {"value_1": true, SHOULD_NOT_EXIST "value_2": "d */},
		{in: `{"" : true, "key2": "value2"}`, want: `{"": true, "key2": "value2"}`, skip: false, strict: false /* {"" : true, "key2": "value2"} */},
		{in: `{""answer"":[{""traits"":''Female aged 60+'',""answer1"":""5""}]}`, want: `{"answer": [{"traits": "Female aged 60+", "answer1": "5"}]}`, skip: false, strict: false /* {""answer"":[{""traits"":''Female aged 60+'',""a */},
		{in: `{ "words": abcdef", "numbers": 12345", "words2": ghijkl" }`, want: `{"words": "abcdef", "numbers": 12345, "words2": "ghijkl"}`, skip: false, strict: false /* { "words": abcdef", "numbers": 12345", "words2": */},
		{in: `{"number": 1,"reason": "According...""ans": "YES"}`, want: `{"number": 1, "reason": "According...", "ans": "YES"}`, skip: false, strict: false /* {"number": 1,"reason": "According...""ans": "YES */},
		{in: `{ "a" : "{ b": {} }" }`, want: `{"a": "{ b"}`, skip: false, strict: false /* { "a" : "{ b": {} }" } */},
		{in: `{"b": "xxxxx" true}`, want: `{"b": "xxxxx"}`, skip: false, strict: false /* {"b": "xxxxx" true} */},
		{in: `{"key": "Lorem "ipsum" s,"}`, want: `{"key": "Lorem \"ipsum\" s,"}`, skip: false, strict: false /* {"key": "Lorem "ipsum" s,"} */},
		{in: `{"lorem": ipsum, sic, datum.",}`, want: `{"lorem": "ipsum, sic, datum."}`, skip: false, strict: false /* {"lorem": ipsum, sic, datum.",} */},
		{in: `{"lorem": sic tamet. "ipsum": sic tamet, quick brown fox. "sic": ipsum}`, want: `{"lorem": "sic tamet.", "ipsum": "sic tamet", "sic": "ipsum"}`, skip: false, strict: false /* {"lorem": sic tamet. "ipsum": sic tamet, quick b */},
		{in: `{"lorem_ipsum": "sic tamet, quick brown fox. }`, want: `{"lorem_ipsum": "sic tamet, quick brown fox."}`, skip: false, strict: false /* {"lorem_ipsum": "sic tamet, quick brown fox. } */},
		{in: `{"key":value, " key2":"value2" }`, want: `{"key": "value", " key2": "value2"}`, skip: false, strict: false /* {"key":value, " key2":"value2" } */},
		{in: `{"key":value "key2":"value2" }`, want: `{"key": "value", "key2": "value2"}`, skip: false, strict: false /* {"key":value "key2":"value2" } */},
		{in: `{'text': 'words{words in brackets}more words'}`, want: `{"text": "words{words in brackets}more words"}`, skip: false, strict: false /* {'text': 'words{words in brackets}more words'} */},
		{in: `{text:words{words in brackets}}`, want: `{"text": "words{words in brackets}"}`, skip: false, strict: false /* {text:words{words in brackets}} */},
		{in: `{text:words{words in brackets}m}`, want: `{"text": "words{words in brackets}m"}`, skip: false, strict: false /* {text:words{words in brackets}m} */},
		{in: "{\"key\": \"value, value2\"```", want: `{"key": "value, value2"}`, skip: false, strict: false /* {"key": "value, value2"``` */},
		{in: "{\"key\": \"value}```", want: `{"key": "value"}`, skip: false, strict: false /* {"key": "value}``` */},
		{in: `{key:value,key2:value2}`, want: `{"key": "value", "key2": "value2"}`, skip: false, strict: false /* {key:value,key2:value2} */},
		{in: `{"key:"value"}`, want: `{"key": "value"}`, skip: false, strict: false /* {"key:"value"} */},
		{in: `{"key:value}`, want: `{"key": "value"}`, skip: false, strict: false /* {"key:value} */},
		{in: `[{"lorem": {"ipsum": "sic"}, """" "lorem": {"ipsum": "sic"}]`, want: `[{"lorem": {"ipsum": "sic"}}, {"lorem": {"ipsum": "sic"}}]`, skip: false, strict: false /* [{"lorem": {"ipsum": "sic"}, """" "lorem": {"ips */},
		{in: `{ "key": ["arrayvalue"], ["arrayvalue1"], ["arrayvalue2"], "key3": "value3" }`, want: `{"key": ["arrayvalue", "arrayvalue1", "arrayvalue2"], "key3": "value3"}`, skip: false, strict: false /* { "key": ["arrayvalue"], ["arrayvalue1"], ["arra */},
		{in: `{ "key": [[1, 2, 3], "a", "b"], [[4, 5, 6], [7, 8, 9]] }`, want: `{"key": [[1, 2, 3], "a", "b", [4, 5, 6], [7, 8, 9]]}`, skip: false, strict: false /* { "key": [[1, 2, 3], "a", "b"], [[4, 5, 6], [7,  */},
		{in: `{ "key": ["arrayvalue"], "key3": "value3", ["arrayvalue1"] }`, want: `{"key": ["arrayvalue"], "key3": "value3", "arrayvalue1": ""}`, skip: false, strict: false /* { "key": ["arrayvalue"], "key3": "value3", ["arr */},
		{in: `{"key": "{\\"key\\\":[\"value\\\"],\"key2\":"value2"}"}`, want: `{"key": "{\"key\":[\"value\"],\"key2\":\"value2\"}"}`, skip: false, strict: false /* {"key": "{\\"key\\\":[\"value\\\"],\"key2\":"val */},
		{in: `{"key": , "key2": "value2"}`, want: `{"key": "", "key2": "value2"}`, skip: false, strict: false /* {"key": , "key2": "value2"} */},
		{in: `{"array":[{"key": "value"], "key2": "value2"}`, want: `{"array": [{"key": "value"}], "key2": "value2"}`, skip: false, strict: false /* {"array":[{"key": "value"], "key2": "value2"} */},
		{in: `[{"key":"value"}},{"key":"value"}]`, want: `[{"key": "value"}, {"key": "value"}]`, skip: false, strict: false /* [{"key":"value"}},{"key":"value"}] */},
		{in: `{'key': ['a':{'duplicated_key': 'duplicated_value', 'duplicated_key': 'duplicated_value'}]}`, want: `{"key": [{"a": {"duplicated_key": "duplicated_value"}}]}`, skip: false, strict: false /* {'key': ['a':{'duplicated_key': 'duplicated_valu */},
		{in: `[{"b":"v2","b":"v2"}]`, want: `[{"b": "v2"}]`, skip: true, strict: false /* [{"b":"v2","b":"v2"}] */},
		{in: `{'item1', 'item2', 'item3'}`, want: `["item1", "item2", "item3"]`, skip: true, strict: false /* {'item1', 'item2', 'item3'} */},
		{in: `{:}`, want: `{}`, skip: true, strict: false /* {:} */},
		{in: `{   }`, want: `{}`, skip: true, strict: false /* {   } */},
		{in: `[{5}s `, want: `[[5]]`, skip: true, strict: false /* [{5}s  */},
		{in: `{"key": "value"}, "key2": "value2"}`, want: `{"key": "value", "key2": "value2"}`, skip: false, strict: false /* {"key": "value"}, "key2": "value2"} */},
		{in: `{"key": "value"}, "key2": }`, want: `{"key": "value", "key2": ""}`, skip: false, strict: false /* {"key": "value"}, "key2": } */},
		{in: `{"key": "value"}, []`, want: `{"key": "value"}`, skip: false, strict: false /* {"key": "value"}, [] */},
		{in: `{"key": "value"}, ["abc"]`, want: `[{"key": "value"}, ["abc"]]`, skip: false, strict: false /* {"key": "value"}, ["abc"] */},
		{in: `{"key": "value"}, {}`, want: `{"key": "value"}`, skip: false, strict: false /* {"key": "value"}, {} */},
		{in: `{"key": "value"}, "" : "value2"}`, want: `{"key": "value", "": "value2"}`, skip: false, strict: false /* {"key": "value"}, "" : "value2"} */},
		{in: `{"key": "value"}, "key2" "value2"}`, want: `{"key": "value", "key2": "value2"}`, skip: false, strict: false /* {"key": "value"}, "key2" "value2"} */},
		{in: `{"key1": "value1"}, "key2": "value2", "key3": "value3"}`, want: `{"key1": "value1", "key2": "value2", "key3": "value3"}`, skip: false, strict: false /* {"key1": "value1"}, "key2": "value2", "key3": "v */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
