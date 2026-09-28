// Code generated from ../json_repair-main/tests/test_parse_string.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestStringParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `stringbeforeobject {}`, want: `{}`, skip: false, strict: false /* stringbeforeobject {} */},
		{in: `{'key': 'string', 'key2': false, "key3": null, "key4": unquoted}`, want: `{"key": "string", "key2": false, "key3": null, "key4": "unquoted"}`, skip: false, strict: false /* {'key': 'string', 'key2': false, "key3": null, " */},
		{in: `{"name": "John", "age": 30, "city": "New York`, want: `{"name": "John", "age": 30, "city": "New York"}`, skip: false, strict: false /* {"name": "John", "age": 30, "city": "New York */},
		{in: `{"name": "John", "age": 30, city: "New York"}`, want: `{"name": "John", "age": 30, "city": "New York"}`, skip: false, strict: false /* {"name": "John", "age": 30, city: "New York"} */},
		{in: `{"name": "John", "age": 30, "city": New York}`, want: `{"name": "John", "age": 30, "city": "New York"}`, skip: false, strict: false /* {"name": "John", "age": 30, "city": New York} */},
		{in: `{"name": John, "age": 30, "city": "New York"}`, want: `{"name": "John", "age": 30, "city": "New York"}`, skip: false, strict: false /* {"name": John, "age": 30, "city": "New York"} */},
		{in: `{“slanted_delimiter”: "value"}`, want: `{"slanted_delimiter": "value"}`, skip: false, strict: false /* {“slanted_delimiter”: "value"} */},
		{in: `{"name": "John", "age": 30, "city": "New`, want: `{"name": "John", "age": 30, "city": "New"}`, skip: false, strict: false /* {"name": "John", "age": 30, "city": "New */},
		{in: `{"name": "John", "age": 30, "city": "New York, "gender": "male"}`, want: `{"name": "John", "age": 30, "city": "New York", "gender": "male"}`, skip: false, strict: false /* {"name": "John", "age": 30, "city": "New York, " */},
		{in: `[{"key": "value", COMMENT "notes": "lorem "ipsum", sic." }]`, want: `[{"key": "value", "notes": "lorem \"ipsum\", sic."}]`, skip: false, strict: false /* [{"key": "value", COMMENT "notes": "lorem "ipsum */},
		{in: `{"key": ""value"}`, want: `{"key": "value"}`, skip: false, strict: false /* {"key": ""value"} */},
		{in: `{"key": "value", 5: "value"}`, want: `{"key": "value", "5": "value"}`, skip: false, strict: false /* {"key": "value", 5: "value"} */},
		{in: `{"foo": "\"bar\""`, want: `{"foo": "\"bar\""}`, skip: false, strict: false /* {"foo": "\"bar\"" */},
		{in: `{"" key":"val"`, want: `{" key": "val"}`, skip: false, strict: false /* {"" key":"val" */},
		{in: `{"key": value "key2" : "value2" `, want: `{"key": "value", "key2": "value2"}`, skip: false, strict: false /* {"key": value "key2" : "value2"  */},
		{in: `{"key": "lorem ipsum ... "sic " tamet. ...}`, want: `{"key": "lorem ipsum ... \"sic \" tamet. ..."}`, skip: false, strict: false /* {"key": "lorem ipsum ... "sic " tamet. ...} */},
		{in: `{"key": value , }`, want: `{"key": "value"}`, skip: false, strict: false /* {"key": value , } */},
		{in: `{"comment": "lorem, "ipsum" sic "tamet". To improve"}`, want: `{"comment": "lorem, \"ipsum\" sic \"tamet\". To improve"}`, skip: false, strict: false /* {"comment": "lorem, "ipsum" sic "tamet". To impr */},
		{in: `{"key": "v"alu"e"} key:`, want: `{"key": "v\"alu\"e"}`, skip: false, strict: false /* {"key": "v"alu"e"} key: */},
		{in: `{"key": "v"alue", "key2": "value2"}`, want: `{"key": "v\"alue", "key2": "value2"}`, skip: false, strict: false /* {"key": "v"alue", "key2": "value2"} */},
		{in: `[{"key": "v"alu,e", "key2": "value2"}]`, want: `[{"key": "v\"alu,e", "key2": "value2"}]`, skip: false, strict: false /* [{"key": "v"alu,e", "key2": "value2"}] */},
		{in: `{"title": ""hello" world"}`, want: `{"title": "\"hello\" world"}`, skip: false, strict: false /* {"title": ""hello" world"} */},
		{in: `{"title": ""hello" world", "b": "y"}`, want: `{"title": "\"hello\" world", "b": "y"}`, skip: false, strict: false /* {"title": ""hello" world", "b": "y"} */},
		{in: `{"title": ""hello" world"}`, want: `{"title": "\"hello\" world"}`, skip: false, strict: true /* {"title": ""hello" world"} */},
		{in: `{"title": ""hello" world", "b": "y"}`, want: `{"title": "\"hello\" world", "b": "y"}`, skip: false, strict: true /* {"title": ""hello" world", "b": "y"} */},
		{in: `{"key": ""value"}`, want: `{"key": "value"}`, skip: false, strict: true /* {"key": ""value"} */},
		{in: `{"a": "first, b: "second"}`, want: `{"a": "first", "b": "second"}`, skip: true, strict: false /* {"a": "first, b: "second"} */},
		{in: `{"a": "first, b: 1}`, want: `{"a": "first", "b": 1}`, skip: true, strict: false /* {"a": "first, b: 1} */},
		{in: `{"a": "first, b: true}`, want: `{"a": "first", "b": true}`, skip: true, strict: false /* {"a": "first, b: true} */},
		{in: `{"a": "first, b: [1]}`, want: `{"a": "first", "b": [1]}`, skip: true, strict: false /* {"a": "first, b: [1]} */},
		{in: `{"a": "first, b: prose}`, want: `{"a": "first", "b": "prose"}`, skip: true, strict: false /* {"a": "first, b: prose} */},
		{in: "{\"key\": 'string\"\n\t\\le'", want: `{"key": "string\"\n\t\\le"}`, skip: false, strict: false /* {"key": 'string"\n \le' */},
		{in: `{"real_content": "Some string: Some other string \t Some string <a href=\"https://domain.com\">Some link</a>"`, want: `{"real_content": "Some string: Some other string \t Some string <a href=\"https://domain.com\">Some link</a>"}`, skip: false, strict: false /* {"real_content": "Some string: Some other string */},
		{in: "{\"key_1\n\": \"value\"}", want: `{"key_1": "value"}`, skip: false, strict: false /* {"key_1\n": "value"} */},
		{in: `{"key	_": "value"}`, want: `{"key\t_": "value"}`, skip: false, strict: false /* {"key _": "value"} */},
		{in: `{"key": 'value'}`, want: `{"key": "value"}`, skip: false, strict: false /* {"key": 'value'} */},
		{in: `{"key": "\u0076\u0061\u006C\u0075\u0065"}`, want: `{"key": "value"}`, skip: true, strict: false /* {"key": "\u0076\u0061\u006C\u0075\u0065"} */},
		{in: `{"key": "valu\'e"}`, want: `{"key": "valu'e"}`, skip: false, strict: false /* {"key": "valu\'e"} */},
		{in: `{'key': "{\"key\": 1, \"key2\": 1}"}`, want: `{"key": "{\"key\": 1, \"key2\": 1}"}`, skip: false, strict: false /* {'key': "{\"key\": 1, \"key2\": 1}"} */},
		{in: `{ "content": "[LINK]("https://google.com")" }`, want: `{"content": "[LINK](\"https://google.com\")"}`, skip: false, strict: false /* { "content": "[LINK]("https://google.com")" } */},
		{in: `{ "content": "[LINK](" }`, want: `{"content": "[LINK]("}`, skip: false, strict: false /* { "content": "[LINK](" } */},
		{in: `{ "content": "[LINK](", "key": true }`, want: `{"content": "[LINK](", "key": true}`, skip: false, strict: false /* { "content": "[LINK](", "key": true } */},
		{in: `{"first": "value", "second": "next"}`, want: `{"first": "value", "second": "next"}`, skip: true, strict: false /* {"first": "value", "second": "next"} */},
		{in: "````{ \"key\": \"value\" }```", want: `{"key": "value"}`, skip: false, strict: false /* ````{ "key": "value" }``` */},
		{in: "{    \"a\": \"\",    \"b\": [ { \"c\": 1} ] \n}```", want: `{"a": "", "b": [{"c": 1}]}`, skip: false, strict: false /* {    "a": "",    "b": [ { "c": 1} ] \n}``` */},
		{in: "Based on the information extracted, here is the filled JSON output: ```json { 'a': 'b' } ```", want: `{"a": "b"}`, skip: false, strict: false /* Based on the information extracted, here is the  */},
		{in: "\n                       The next 64 elements are:\n                       ```json\n                       { \"key\": \"value\" }\n                       ```", want: `{"key": "value"}`, skip: false, strict: false /* \n                       The next 64 elements are */},
		{in: "{\"key\": \"``\"", want: "{\"key\": \"``\"}", skip: false, strict: false /* {"key": "``" */},
		{in: "{\"key\": \"```json\"", want: "{\"key\": \"```json\"}", skip: false, strict: false /* {"key": "```json" */},
		{in: "{\"key\": \"```json {\"key\": [{\"key1\": 1},{\"key2\": 2}]}```\"}", want: `{"key": {"key": [{"key1": 1}, {"key2": 2}]}}`, skip: false, strict: false /* {"key": "```json {"key": [{"key1": 1},{"key2": 2 */},
		{in: "{\"response\": \"```json{}\"", want: "{\"response\": \"```json{}\"}", skip: false, strict: false /* {"response": "```json{}" */},
		{in: "{\"key\": \"```json nope\\n\"}", want: "{\"key\": \"```json nope\"}", skip: true, strict: false /* {"key": "```json nope\n"} */},
		{in: `  {"key": true, "key2": false, "key3": null}`, want: `{"key": true, "key2": false, "key3": null}`, skip: false, strict: false /*   {"key": true, "key2": false, "key3": null} */},
		{in: `{"key": TRUE, "key2": FALSE, "key3": Null}   `, want: `{"key": true, "key2": false, "key3": null}`, skip: false, strict: false /* {"key": TRUE, "key2": FALSE, "key3": Null}    */},
		{in: `{"value": None }`, want: `{"value": null}`, skip: false, strict: false /* {"value": None } */},
		{in: `[None, TRUE, false, Null]`, want: `[null, true, false, null]`, skip: false, strict: false /* [None, TRUE, false, Null] */},
		{in: `[None`, want: `[null]`, skip: false, strict: false /* [None */},
		{in: `{"value": "None"}`, want: `{"value": "None"}`, skip: false, strict: false /* {"value": "None"} */},
		{in: `{"value": none}`, want: `{"value": null}`, skip: false, strict: false /* {"value": none} */},
		{in: `{"value": NONE}`, want: `{"value": null}`, skip: false, strict: false /* {"value": NONE} */},
		{in: `{"value": trueblue}`, want: `{"value": "trueblue"}`, skip: false, strict: false /* {"value": trueblue} */},
		{in: `{"value": falsehood}`, want: `{"value": "falsehood"}`, skip: false, strict: false /* {"value": falsehood} */},
		{in: `{"value": nullify}`, want: `{"value": "nullify"}`, skip: false, strict: false /* {"value": nullify} */},
		{in: `{"value": NoneType}`, want: `{"value": "NoneType"}`, skip: false, strict: false /* {"value": NoneType} */},
		{in: `{"value": None}`, want: `{"value": null}`, skip: false, strict: false /* {"value": None} */},
		{in: `{"key": "value", "items": ["alpha", "beta"]}`, want: `{"key": "value", "items": ["alpha", "beta"]}`, skip: false, strict: false /* {"key": "value", "items": ["alpha", "beta"]} */},
		{in: `{"key": "a\\\\b\\\\c",}`, want: `{"key": "a\\\\b\\\\c"}`, skip: false, strict: false /* {"key": "a\\\\b\\\\c",} */},
		{in: `{"key": "1\\\\2\\\\3",}`, want: `{"key": "1\\\\2\\\\3"}`, skip: false, strict: false /* {"key": "1\\\\2\\\\3",} */},
		{in: `{0:a"0"`, want: `{"0": "a"}`, skip: true, strict: false /* {0:a"0" */},
		{in: `{'': 1}`, want: `{"": 1}`, skip: false, strict: false /* {'': 1} */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
