package json_repair

import (
	"strconv"
	"testing"
)

// TestPythonFloatString round-trips Python's str()/repr() float formatting:
// each expected string is parsed back to the exact float64 and must be
// re-rendered identically by pythonFloatString.
func TestPythonFloatString(t *testing.T) {
	expected := []string{
		"1.0", "1.5", "0.25", "1234.56", "85.6", "10000000000.0",
		"1000000000000000.0", "1e+16", "0.0001", "1e-05", "0.1",
		"0.30000000000000004", "1.7976931348623157e+308", "5e-324", "2.5",
		"100.0", "123456.789", "1e-06", "123456789.0", "9007199254740992.0",
		"-1.0", "-0.0", "-123456.789", "-1e-05", "3.141592653589793",
		"1.2250967970040019e-186", "3.2040266842312833e-19", "7.628240927476104e-158",
		"1.7824494936647445e+258", "1.286044115336052e+132", "2.967741773232428e-205",
		"6.418168561449817e+217", "2.789538856179834e+274", "4.775678388654925e+258",
		"6.303391154883172e+159", "7.829266304167897e-294", "7.283254554894034e-137",
		"3.5008420750447736e+48", "9.61491764986102e-80", "1.9198924886786372e+44",
		"1.870447391501175e+89", "6.433534282302013e+52", "7.567586080244352e-256",
		"2.1234354656788824e+249", "1.7092017827061227e+87", "8.464641978276944",
		"6.196169307310853e+70", "1.4124194529009596e-229", "7.957615067098218e-67",
		"8.697859489136313e-219", "4.421136025352491e-197", "6.721159998379595e+164",
		"2.4638868744047616e+73", "2.885563276943387e+63", "7.316382821126955e-27",
		"6.482179051002887e-227", "5.807258893267365e-125", "2.4706224438573543e-50",
		"3.429530441058499e+88", "2.9765365657324186e+270", "8.585667281708277e+32",
		"3.061432647676937e-243", "8.245412206940919e-268", "3.4096678837813216e+110",
		"9.218225117669895e-84",
	}
	for _, want := range expected {
		f, err := strconv.ParseFloat(want, 64)
		if err != nil {
			t.Fatalf("ParseFloat(%q): %v", want, err)
		}
		if got := pythonFloatString(f); got != want {
			t.Errorf("pythonFloatString(%g):\n got  %q\n want %q", f, got, want)
		}
	}
}

func mustDump(t *testing.T, v any, opts DumpOptions) string {
	t.Helper()
	s, err := Dump(v, opts)
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	return s
}

func TestDumpStrings(t *testing.T) {
	in := `中文 "q" \ tab` + "\t" + ` new` + "\n" + ` ctrl` + "\x01" + ` astral😀`
	// ensure_ascii=true (Python json.dumps default): non-ASCII + control escaped.
	wantASCII := `"\u4e2d\u6587 \"q\" \\ tab\t new\n ctrl\u0001 astral\ud83d\ude00"`
	if got := mustDump(t, in, DumpOptions{EnsureASCII: true}); got != wantASCII {
		t.Fatalf("ensure_ascii=true:\n got  %s\n want %s", got, wantASCII)
	}
	// ensure_ascii=false: non-ASCII kept literal, control chars still escaped.
	wantUTF8 := `"中文 \"q\" \\ tab\t new\n ctrl\u0001 astral😀` + `"`
	if got := mustDump(t, in, DumpOptions{EnsureASCII: false}); got != wantUTF8 {
		t.Fatalf("ensure_ascii=false:\n got  %s\n want %s", got, wantUTF8)
	}
}

func TestDumpIndent(t *testing.T) {
	// Keys are already in sorted order so Go's sorted-key output matches the
	// Python (insertion-order) expectation here.
	v := map[string]any{
		"a": map[string]any{"c": []any{int64(1), int64(2), "x"}},
		"b": int64(1),
	}
	if got := mustDump(t, v, DumpOptions{}); got != `{"a": {"c": [1, 2, "x"]}, "b": 1}` {
		t.Fatalf("compact:\n got  %s\n want %s", got, `{"a": {"c": [1, 2, "x"]}, "b": 1}`)
	}
	want0 := "{\n\"a\": {\n\"c\": [\n1,\n2,\n\"x\"\n]\n},\n\"b\": 1\n}"
	if got := mustDump(t, v, DumpOptions{Pretty: true, Indent: 0}); got != want0 {
		t.Fatalf("indent0:\n got  %q\n want %q", got, want0)
	}
	want2 := "{\n  \"a\": {\n    \"c\": [\n      1,\n      2,\n      \"x\"\n    ]\n  },\n  \"b\": 1\n}"
	if got := mustDump(t, v, DumpOptions{Pretty: true, Indent: 2}); got != want2 {
		t.Fatalf("indent2:\n got  %q\n want %q", got, want2)
	}
	e := map[string]any{"a": []any{}, "b": map[string]any{}, "c": nil, "d": true}
	if got := mustDump(t, e, DumpOptions{Pretty: true, Indent: 4}); got != "{\n    \"a\": [],\n    \"b\": {},\n    \"c\": null,\n    \"d\": true\n}" {
		t.Fatalf("indent4:\n got  %q", got)
	}
}
