package json_repair

import (
	"testing"
)

func TestCommentBracketSafety(t *testing.T) {
	// Line comment containing ] should NOT trigger empty-object→array fallback.
	{
		got, _, err := RepairJSON("{\n// comment ]\n}", Options{SkipJsonLoads: true, Logging: true})
		if err != nil {
			t.Fatalf("RepairJSON: %v", err)
		}
		assertEq(t, got, map[string]any{})
	}
	// Block comment containing ] should NOT trigger empty-object→array fallback.
	{
		got, _, err := RepairJSON("{/* comment ] */}", Options{SkipJsonLoads: true, Logging: true})
		if err != nil {
			t.Fatalf("RepairJSON: %v", err)
		}
		assertEq(t, got, map[string]any{})
	}
}

func TestCommentInArrayItems(t *testing.T) {
	raw := `
	{
		"Changes": [
			//object a
			{
				"Action": "1"
			},
			//object b ]
			{
				"Action": "2"
			},
			//object c ]
			{
				"Action": "3"
			}
		]
	}
	`
	got, _, err := RepairJSON(raw, Options{SkipJsonLoads: true})
	if err != nil {
		t.Fatalf("RepairJSON: %v", err)
	}
	want := map[string]any{
		"Changes": []any{
			map[string]any{"Action": "1"},
			map[string]any{"Action": "2"},
			map[string]any{"Action": "3"},
		},
	}
	assertEq(t, got, want)
}

func TestStrictMode(t *testing.T) {
	errCases := []struct {
		in   string
		want string
	}{
		{`{"key":"value"}["value"]`, "Multiple top-level"},
		{`{"key":"value"}, {"key":"value_after"}`, "Multiple top-level"},
		{`{"key":"value"}{"key":"value_after"}`, "Multiple top-level"},
		{`[1][2]`, "Multiple top-level"},
		{`{"key":"value"}[]`, "Multiple top-level"},
		{`{"key":"value"}{}`, "Multiple top-level"},
		{`[]{}`, "Multiple top-level"},
		{`{}[]`, "Multiple top-level"},
		{`[1]{}`, "Multiple top-level"},
		{`[{"key": "first", "key": "second"}]`, "Duplicate key"},
		{`{"" : "value"}`, "Empty key"},
		{`{"missing" "colon"}`, "Missing ':'"},
		{`{"key": , "key2": "value2"}`, "empty"},
		{`{"dangling"}`, "empty"},
		{`{"key": """"}`, "doubled quotes"},
		{`{"key": "" "value"}`, "doubled quotes"},
		{`{\"key\": \"value\"}`, "empty"},
	}
	for _, c := range errCases {
		t.Run(c.in, func(t *testing.T) {
			_, _, err := RepairJSON(c.in, Options{SkipJsonLoads: true, Strict: true})
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.want)
			}
		})
	}
}
