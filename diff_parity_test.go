package json_repair

// Differential parity check against the REAL Python implementation.
//
// This is the strongest 1:1 verification: the same corpus of inputs is fed to
// both the Python original (reference produced by tools/diff_check/py_ref.py,
// which runs ../json_repair-main/src/json_repair) and this Go port, and the
// normalized outputs are compared byte-for-byte. Both sides emit object keys
// in Python dict insertion order (D-006: Go Dump of *OrderedMap vs Python
// json.dumps without sort_keys), so key ORDER is part of the comparison.
// Schema combos pass diffSchemaOrdered() (an *OrderedMap with the same
// property order as py_ref.py's SCHEMA_PERF) because schema property order is
// input state the repair rebuilds on. It catches what the
// hand-translated unit tests structurally cannot: a transcription error in a
// translated test's expected value.
//
// Setup (local dev only — needs the sibling Python repo + oracle venv):
//
//	.venv-oracle/bin/python tools/diff_check/gen_corpus.py
//	.venv-oracle/bin/python tools/diff_check/py_ref.py
//	DIFF_CHECK=1 go test -run TestDiffParity -v .
//
// The test is gated behind DIFF_CHECK=1 because it is a heavy harness
// (37k repairs); the default `go test ./...` stays fast. It skips when the
// generated files are absent.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type diffRefLine struct {
	I int    `json:"i"`
	C string `json:"c"`
	O string `json:"o"`
}

func diffComboOptions(name string) (Options, bool) {
	switch name {
	case "skipF_ssF":
		return Options{ReturnObjects: true}, true
	case "skipT_ssF":
		return Options{ReturnObjects: true, SkipJsonLoads: true}, true
	case "skipF_ssT":
		return Options{ReturnObjects: true, StreamStable: true}, true
	case "skipT_ssT":
		return Options{ReturnObjects: true, SkipJsonLoads: true, StreamStable: true}, true
	case "schema_skipF":
		return Options{ReturnObjects: true, Schema: diffSchemaOrdered()}, true
	case "schema_skipT":
		return Options{ReturnObjects: true, SkipJsonLoads: true, Schema: diffSchemaOrdered()}, true
	}
	return Options{}, false
}

// diffSchemaOrdered mirrors py_ref.py's SCHEMA_PERF including property
// INSERTION ORDER — a plain Go map literal cannot express it (it would be
// normalized with sorted keys), and the schema property order is part of the
// input: repairObject rebuilds objects in schema property order (D-006), so
// both sides must see the same order for the comparison to be a true 1:1.
func diffSchemaOrdered() *OrderedMap {
	return orderedSchema(
		"type", "array",
		"items", orderedSchema(
			"type", "object",
			"properties", orderedSchema(
				"_id", orderedSchema("type", "string"),
				"index", orderedSchema("type", "integer"),
				"guid", orderedSchema("type", "string"),
				"isActive", orderedSchema("type", "boolean"),
				"tags", orderedSchema("type", "array", "items", orderedSchema("type", "string")),
				"friends", orderedSchema("type", "array", "items", orderedSchema(
					"type", "object",
					"properties", orderedSchema(
						"id", orderedSchema("type", "integer"),
						"name", orderedSchema("type", "string"),
					),
					"required", []any{"id", "name"},
				)),
			),
			"required", []any{"_id", "index", "guid", "isActive", "tags", "friends"},
			"additionalProperties", true,
		),
	)
}

func TestDiffParity(t *testing.T) {
	if os.Getenv("DIFF_CHECK") == "" {
		t.Skip("set DIFF_CHECK=1 to run the differential parity harness " +
			"(after generating tools/diff_check/corpus.jsonl + py_ref.jsonl)")
	}
	dir := "tools/diff_check"
	corpusPath := filepath.Join(dir, "corpus.jsonl")
	refPath := filepath.Join(dir, "py_ref.jsonl")
	if _, err := os.Stat(refPath); err != nil {
		t.Skip("py_ref.jsonl not found; run .venv-oracle/bin/python tools/diff_check/gen_corpus.py " +
			"and tools/diff_check/py_ref.py first")
	}

	corpus, err := readDiffCorpus(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	refs, err := readDiffRefs(refPath)
	if err != nil {
		t.Fatalf("read refs: %v", err)
	}
	if len(refs) != len(corpus)*6 {
		t.Fatalf("stale py_ref.jsonl: %d lines, want %d (len(corpus)*6); regenerate", len(refs), len(corpus)*6)
	}

	// 19+ consecutive digits: outside int64 — a documented type-mapping limit
	// (D-002), not a repair-behavior bug.
	bigIntTok := regexp.MustCompile(`-?\d{19,}`)

	type diffEntry struct {
		combo, index, input, goOut, pyOut string
	}
	var (
		fatalOutput   []diffEntry // both OK, different values
		fatalStatus   []diffEntry // exactly one side errored
		fatalErrText  []diffEntry // both errored, non-schema, different messages
		int64Limit    []diffEntry // value mismatch involving >int64 integers
		schemaErrText []diffEntry // schema combos, both errored, different messages (informational)
	)
	report := func(kind string, e diffEntry) {
		switch kind {
		case "output":
			fatalOutput = append(fatalOutput, e)
		case "status":
			fatalStatus = append(fatalStatus, e)
		case "errtext":
			fatalErrText = append(fatalErrText, e)
		case "int64":
			int64Limit = append(int64Limit, e)
		case "schemaerr":
			schemaErrText = append(schemaErrText, e)
		}
	}

	for _, r := range refs {
		if r.I < 0 || r.I >= len(corpus) {
			t.Fatalf("ref line index %d out of corpus range %d", r.I, len(corpus))
		}
		opts, ok := diffComboOptions(r.C)
		if !ok {
			t.Fatalf("unknown combo %q (py_ref.py and this test drifted?)", r.C)
		}
		input := corpus[r.I]
		v, _, rerr := RepairJSON(input, opts)
		var got string
		if rerr != nil {
			got = "ERR:" + rerr.Error()
		} else {
			s, derr := Dump(v, DumpOptions{EnsureASCII: false})
			if derr != nil {
				got = "ERR:Dump:" + derr.Error()
			} else {
				got = s
			}
		}
		isSchema := strings.HasPrefix(r.C, "schema_")
		cmpGot, want := got, r.O
		if isSchema {
			cmpGot, want = normalizeSchemaErr(cmpGot), normalizeSchemaErr(want)
		}
		if cmpGot == want {
			continue
		}
		gotErr := strings.HasPrefix(got, "ERR:")
		wantErr := strings.HasPrefix(r.O, "ERR:")
		entry := diffEntry{
			combo: r.C,
			index: strconv.Itoa(r.I),
			input: diffExcerpt(input),
			goOut: diffExcerpt(got),
			pyOut: diffExcerpt(r.O),
		}
		switch {
		case gotErr && wantErr:
			if isSchema {
				report("schemaerr", entry)
			} else {
				report("errtext", entry)
			}
		case gotErr != wantErr:
			report("status", entry)
		case bigIntTok.MatchString(r.O) || bigIntTok.MatchString(got):
			report("int64", entry)
		default:
			report("output", entry)
		}
	}

	// Write the full report for review.
	var b strings.Builder
	writeSection := func(title string, entries []diffEntry, max int) {
		b.WriteString("== " + title + " (" + strconv.Itoa(len(entries)) + ")\n")
		for i, e := range entries {
			if i >= max {
				b.WriteString("  ... " + strconv.Itoa(len(entries)-max) + " more ...\n")
				break
			}
			b.WriteString("  [" + e.combo + " #" + e.index + "]\n")
			b.WriteString("    in : " + e.input + "\n")
			b.WriteString("    go : " + e.goOut + "\n")
			b.WriteString("    py : " + e.pyOut + "\n")
		}
	}
	writeSection("FATAL: output mismatch (both OK, different value)", fatalOutput, 100)
	writeSection("FATAL: status mismatch (one side errored)", fatalStatus, 100)
	writeSection("FATAL: error-text mismatch (non-schema)", fatalErrText, 100)
	writeSection("KNOWN LIMIT: >int64 integers (D-002)", int64Limit, 20)
	writeSection("INFO: schema error-text differs (status matched)", schemaErrText, 20)
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("report written to %s/report.txt", dir)
	t.Logf("output=%d status=%d errtext=%d int64=%d schemaerr=%d",
		len(fatalOutput), len(fatalStatus), len(fatalErrText), len(int64Limit), len(schemaErrText))

	fatal := len(fatalOutput) + len(fatalStatus) + len(fatalErrText)
	if fatal > 0 {
		t.Fatalf("differential parity: %d fatal mismatches (see %s/report.txt)", fatal, dir)
	}
}

func readDiffCorpus(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var s string
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func readDiffRefs(path string) ([]diffRefLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []diffRefLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var r diffRefLine
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

func diffExcerpt(s string) string {
	r := []rune(s)
	if len(r) > 120 {
		return string(r[:120]) + "..."
	}
	return s
}

// normalizeSchemaErr removes the two artifacts that make schema-combo error
// text differ without any behavior difference:
//   - Python raises ValueError and py_ref.py records "ERR:ValueError:<msg>"
//     while Go returns the bare "ERR:<msg>";
//   - Python iterates a set[str] of required keys, so the missing-keys list
//     order is hash-seed dependent (verified: PYTHONHASHSEED changes it),
//     while Go sorts it deterministically.
var missingRequiredRe = regexp.MustCompile(`^(Missing required properties at .+?): (.+)$`)

func normalizeSchemaErr(s string) string {
	if !strings.HasPrefix(s, "ERR:") {
		return s
	}
	msg := strings.TrimPrefix(strings.TrimPrefix(s, "ERR:"), "ValueError:")
	if m := missingRequiredRe.FindStringSubmatch(msg); m != nil {
		parts := strings.Split(m[2], ", ")
		sort.Strings(parts)
		return "ERR:" + m[1] + ": " + strings.Join(parts, ", ")
	}
	return "ERR:" + msg
}
