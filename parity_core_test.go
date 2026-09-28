// Code generated from ../json_repair-main/tests/test_json_repair.py by tools/gen_parity_test.py. DO NOT EDIT BY HAND.
package json_repair

import (
	"testing"
)

func TestCoreParity(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		skip   bool
		strict bool
	}{
		{in: `{"name": "John", "age": 30, "city": "New York"}`, want: `{"name": "John", "age": 30, "city": "New York"}`, skip: false, strict: false /* {"name": "John", "age": 30, "city": "New York"} */},
		{in: `{"employees":["John", "Anna", "Peter"]} `, want: `{"employees": ["John", "Anna", "Peter"]}`, skip: false, strict: false /* {"employees":["John", "Anna", "Peter"]}  */},
		{in: `{"key": "value:value"}`, want: `{"key": "value:value"}`, skip: false, strict: false /* {"key": "value:value"} */},
		{in: `{"text": "The quick brown fox,"}`, want: `{"text": "The quick brown fox,"}`, skip: false, strict: false /* {"text": "The quick brown fox,"} */},
		{in: `{"text": "The quick brown fox won't jump"}`, want: `{"text": "The quick brown fox won't jump"}`, skip: false, strict: false /* {"text": "The quick brown fox won't jump"} */},
		{in: `{"key": ""`, want: `{"key": ""}`, skip: false, strict: false /* {"key": "" */},
		{in: `{"key1": {"key2": [1, 2, 3]}}`, want: `{"key1": {"key2": [1, 2, 3]}}`, skip: false, strict: false /* {"key1": {"key2": [1, 2, 3]}} */},
		{in: `{"key": 12345678901234567890}`, want: `{"key": 12345678901234567890}`, skip: false, strict: false /* {"key": 12345678901234567890} */},
		{in: `{"key": "value☺"}`, want: `{"key": "value☺"}`, skip: false, strict: false /* {"key": "value☺"} */},
		{in: `{"key": "value\nvalue"}`, want: `{"key": "value\nvalue"}`, skip: false, strict: false /* {"key": "value\nvalue"} */},
		{in: `{"items": [1, 2, 3]}`, want: `{"items": [1, 2, 3]}`, skip: true, strict: false /* {"items": [1, 2, 3]} */},
		{in: `Here is your JSON: {"key": "value`, want: `{"key": "value"}`, skip: false, strict: false /* Here is your JSON: {"key": "value */},
		{in: `[]{}`, want: `[]`, skip: false, strict: false /* []{} */},
		{in: `[]{"key":"value"}`, want: `{"key": "value"}`, skip: false, strict: false /* []{"key":"value"} */},
		{in: `{"key":"value"}[1,2,3,True]`, want: `[{"key": "value"}, [1, 2, 3, true]]`, skip: false, strict: false /* {"key":"value"}[1,2,3,True] */},
		{in: `{"key":"value"}, {"key":"value_after"}`, want: `[{"key": "value"}, {"key": "value_after"}]`, skip: false, strict: false /* {"key":"value"}, {"key":"value_after"} */},
		{in: "lorem ```json {\"key\":\"value\"} ``` ipsum ```json [1,2,3,True] ``` 42", want: `[{"key": "value"}, [1, 2, 3, true]]`, skip: false, strict: false /* lorem ```json {"key":"value"} ``` ipsum ```json  */},
		{in: `[{"key":"value"}][{"key":"value_after"}]`, want: `[{"key": "value_after"}]`, skip: false, strict: false /* [{"key":"value"}][{"key":"value_after"}] */},
		{in: "\n         **Decision**: bla, bla (some clarification):\n\n        ```json\n        {\n          \"key\": \"value\"\n        }\n        ```\n        ", want: `{"key": "value"}`, skip: false, strict: false /* \n         **Decision**: bla, bla (some clarifica */},
		{in: "\n        (1) Keep this note in the explanation.\n\n        ```json\n        {\n          \"key\": \"value\"\n        }\n        ```\n        ", want: `{"key": "value"}`, skip: false, strict: false /* \n        (1) Keep this note in the explanation.\n */},
		{in: "\n        Here is the tuple payload:\n\n        ```json\n        (1, 2)\n        ```\n        ", want: `[1, 2]`, skip: false, strict: false /* \n        Here is the tuple payload:\n\n        ``` */},
		{in: `[]`, want: `[]`, skip: false, strict: false /* [] */},
		{in: `{}`, want: `{}`, skip: false, strict: false /* {} */},
		{in: `{"key": true, "key2": false, "key3": null}`, want: `{"key": true, "key2": false, "key3": null}`, skip: false, strict: false /* {"key": true, "key2": false, "key3": null} */},
		{in: `[1, 2, 3, 4]`, want: `[1, 2, 3, 4]`, skip: false, strict: false /* [1, 2, 3, 4] */},
		{in: "\n{\n  \"resourceType\": \"Bundle\",\n  \"id\": \"1\",\n  \"type\": \"collection\",\n  \"entry\": [\n    {\n      \"resource\": {\n        \"resourceType\": \"Patient\",\n        \"id\": \"1\",\n        \"name\": [\n          {\"use\": \"official\", \"family\": \"Corwin\", \"given\": [\"Keisha\", \"Sunny\"], \"prefix\": [\"Mrs.\"},\n          {\"use\": \"maiden\", \"family\": \"Goodwin\", \"given\": [\"Keisha\", \"Sunny\"], \"prefix\": [\"Mrs.\"]}\n        ]\n      }\n    }\n  ]\n}\n", want: `{"resourceType": "Bundle", "id": "1", "type": "collection", "entry": [{"resource": {"resourceType": "Patient", "id": "1", "name": [{"use": "official", "family": "Corwin", "given": ["Keisha", "Sunny"], "prefix": ["Mrs."]}, {"use": "maiden", "family": "Goodwin", "given": ["Keisha", "Sunny"], "prefix": ["Mrs."]}]}}]}`, skip: false, strict: false /* \n{\n  "resourceType": "Bundle",\n  "id": "1",\n  "t */},
		{in: "{\n\"html\": \"<h3 id=\"aaa\">Waarom meer dan 200 Technical Experts - \"Passie voor techniek\"?</h3>\"}", want: `{"html": "<h3 id=\"aaa\">Waarom meer dan 200 Technical Experts - \"Passie voor techniek\"?</h3>"}`, skip: false, strict: false /* {\n"html": "<h3 id="aaa">Waarom meer dan 200 Tech */},
		{in: "\n        [\n            {\n                \"foo\": \"Foo bar baz\",\n                \"tag\": \"#foo-bar-baz\"\n            },\n            {\n                \"foo\": \"foo bar \"foobar\" foo bar baz.\",\n                \"tag\": \"#foo-bar-foobar\"\n            }\n        ]\n        ", want: `[{"foo": "Foo bar baz", "tag": "#foo-bar-baz"}, {"foo": "foo bar \"foobar\" foo bar baz.", "tag": "#foo-bar-foobar"}]`, skip: false, strict: false /* \n        [\n            {\n                "foo":  */},
		{in: `{"key": true, "key2": false, "key3": null}`, want: `{"key": true, "key2": false, "key3": null}`, skip: true, strict: false /* {"key": true, "key2": false, "key3": null} */},
		{in: `{"key": true, "key2": false, "key3": }`, want: `{"key": true, "key2": false, "key3": ""}`, skip: true, strict: false /* {"key": true, "key2": false, "key3": } */},
		{in: `{'test_中国人_ascii':'统一码'}`, want: `{"test_中国人_ascii": "统一码"}`, skip: false, strict: false /* {'test_中国人_ascii':'统一码'} */},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assertRepair(t, c.in, c.want, c.skip, c.strict)
		})
	}
}
