//go:build !race

// Translated from the authoritative Python performance suite
// (../json_repair-main/tests/test_performance.py, 16 cases).
//
// The Python suite is the behavioral/performance spec. It skips in CI
// (`@pytest.mark.skipif(CI, ...)`), so the Go port mirrors that: the
// threshold test below is skipped when the CI env var is present or when run
// with `-short`. The caps are 10x the CPython thresholds (documented per case)
// — Go's parser is linear and far faster than CPython, so the margin is there
// to catch *algorithmic* (order-of-magnitude) regressions, not constant-factor
// drift. The `Benchmark*` functions give per-case ns/op for `go test -bench`
// trend comparison over time.
//
// The whole file is excluded under `-race` (//go:build !race above): race
// instrumentation slows the code ~10x, which would blow the already-10x caps
// and make `go test -race ./...` fail spuriously. Timing is meaningless under
// the race detector anyway, so the thresholds are simply not run there.
package json_repair

import (
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Package-level sinks so the compiler cannot eliminate the repair work done in
// the benchmark/threshold loop bodies.
var (
	perfSinkValue  any
	perfSinkString string
)

// perfCase describes one repair workload and its performance cap.
type perfCase struct {
	name      string
	input     string
	opts      Options
	useString bool // true → exercise RepairJSONString (return_objects=False path)
	maxTime   time.Duration
}

// perfSchema mirrors the Python schema_perf dict.
func perfSchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"_id":      map[string]any{"type": "string"},
				"index":    map[string]any{"type": "integer"},
				"guid":     map[string]any{"type": "string"},
				"isActive": map[string]any{"type": "boolean"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"friends": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "integer"},
							"name": map[string]any{"type": "string"},
						},
						"required": []any{"id", "name"},
					},
				},
			},
			"required":             []any{"_id", "index", "guid", "isActive", "tags", "friends"},
			"additionalProperties": true,
		},
	}
}

// unclosedObjectStringPayload mirrors Python _unclosed_object_string_payload.
// All fragments are ASCII, so byte length == character length here.
func unclosedObjectStringPayload(target int, factory func(int) string) string {
	base := `{"a": "`
	var pieces []string
	index := 0
	for len(base)+len(strings.Join(pieces, ",")) < target {
		pieces = append(pieces, factory(index))
		index++
	}
	return base + strings.Join(pieces, ",")
}

// unclosedObjectStringFragments mirrors the Python 3000-fragment literal.
func unclosedObjectStringFragments() string {
	parts := make([]string, 3000)
	for i := range parts {
		parts[i] = "fragment"
	}
	return `{"a": "` + strings.Join(parts, ",")
}

// perfLogger is the minimal surface shared by *testing.T and *testing.B.
type perfLogger interface {
	Helper()
	Fatalf(format string, args ...any)
}

func readFile(t perfLogger, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// perfCases builds the 16 workloads, mirroring test_performance.py 1:1.
// Caps are 10x the CPython threshold for the corresponding Python test.
func perfCases(t perfLogger) []perfCase {
	t.Helper()
	valid := readFile(t, "testdata/valid.json")
	invalid := readFile(t, "testdata/invalid.json")

	mixed := unclosedObjectStringPayload(35000, func(index int) string {
		switch index % 3 {
		case 0:
			return `frag"ment`
		case 1:
			return "'fragment'"
		default:
			return "fragment"
		}
	})
	farComma := `{"a": "` + strings.Repeat("x,", 10000) + `" tail`
	farBrace := `{"a": "` + strings.Repeat("x}", 5000) + `" tail`

	ms := time.Millisecond
	us := time.Microsecond
	// String-output cases set EnsureASCII=true to mirror Python's
	// repair_json(return_objects=False) default of ensure_ascii=True.
	return []perfCase{
		{name: "true_true_correct", input: valid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 3 * ms}, // python 3ms
		{name: "true_true_incorrect", input: invalid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 3 * ms},
		{name: "true_false_correct", input: valid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: false},
			maxTime: 10 * 30 * us}, // python 30us (stdlib fast path)
		{name: "true_false_incorrect", input: invalid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: false},
			maxTime: 10 * 3 * ms},
		{name: "false_true_correct", input: valid, useString: true,
			opts:    Options{SkipJsonLoads: true, EnsureASCII: true},
			maxTime: 10 * 3 * ms},
		{name: "false_true_incorrect", input: invalid, useString: true,
			opts:    Options{SkipJsonLoads: true, EnsureASCII: true},
			maxTime: 10 * 3 * ms},
		{name: "false_false_correct", input: valid, useString: true,
			opts:    Options{SkipJsonLoads: false, EnsureASCII: true},
			maxTime: 10 * 60 * us}, // python 60us (stdlib fast path)
		{name: "false_false_incorrect", input: invalid, useString: true,
			opts:    Options{SkipJsonLoads: false, EnsureASCII: true},
			maxTime: 10 * 3 * ms},
		{name: "unclosed_object_string_with_many_comma_fragments",
			input:   unclosedObjectStringFragments(),
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 75 * ms}, // python 75ms
		{name: "unclosed_object_string_with_mixed_quote_fragments",
			input:   mixed,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 125 * ms}, // python 125ms
		{name: "far_quote_object_string_with_many_comma_fragments",
			input:   farComma,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 250 * ms}, // python 250ms
		{name: "far_quote_object_string_with_many_brace_fragments",
			input:   farBrace,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true},
			maxTime: 10 * 250 * ms}, // python 250ms
		{name: "schema_true_false_correct", input: valid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: false, Schema: perfSchema()},
			maxTime: 10 * 600 * us}, // python 600us
		{name: "schema_false_false_correct", input: valid, useString: true,
			opts:    Options{SkipJsonLoads: false, EnsureASCII: true, Schema: perfSchema()},
			maxTime: 10 * 800 * us}, // python 800us
		{name: "schema_true_true_incorrect", input: invalid,
			opts:    Options{ReturnObjects: true, SkipJsonLoads: true, Schema: perfSchema()},
			maxTime: 10 * 4500 * us}, // python 4.5ms
		{name: "schema_false_true_incorrect", input: invalid, useString: true,
			opts:    Options{SkipJsonLoads: true, EnsureASCII: true, Schema: perfSchema()},
			maxTime: 10 * 4500 * us}, // python 4.5ms
	}
}

// runPerfCase executes one workload, routing to the correct API and sinking the
// result so it cannot be optimized away.
func runPerfCase(c perfCase) {
	if c.useString {
		s, _, _ := RepairJSONString(c.input, c.opts)
		perfSinkString = s
		return
	}
	v, _, _ := RepairJSON(c.input, c.opts)
	perfSinkValue = v
}

// BenchmarkRepairPerformance is the trend net: run `go test -run=NONE -bench=.
// -benchmem` to compare ns/op and allocs/op over time.
func BenchmarkRepairPerformance(b *testing.B) {
	cases := perfCases(b)
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				runPerfCase(c)
			}
		})
	}
}

// medianDuration runs f after one warm-up, then returns the median of `runs`
// measured durations (mirrors pytest-benchmark's median; guards against a
// single GC/allocation spike flaking the threshold).
func medianDuration(runs int, f func()) time.Duration {
	f() // warm-up (page faults, first GC, etc.)
	durs := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		f()
		durs = append(durs, time.Since(start))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	return durs[len(durs)/2]
}

// TestPerformanceRegression is the failing net for local runs. Like the Python
// suite it is skipped in CI (CI env var present) and under `-short`; on a dev
// machine it asserts each workload stays under its 10x-CPython cap.
func TestPerformanceRegression(t *testing.T) {
	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("performance regression tests are skipped in CI")
	}
	if testing.Short() {
		t.Skip("performance regression tests are skipped under -short")
	}
	cases := perfCases(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			median := medianDuration(5, func() { runPerfCase(c) })
			if median >= c.maxTime {
				t.Errorf("performance regression: %s took %s (median of 5), cap %s",
					c.name, median, c.maxTime)
			} else {
				t.Logf("%s: %s (cap %s)", c.name, median, c.maxTime)
			}
		})
	}
}
