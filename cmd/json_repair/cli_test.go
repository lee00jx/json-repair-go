package main

// Translated from the authoritative Python test suite
// (../json_repair-main/tests/test_repair_json_cli.py). The pydantic-model
// case (test_cli_schema_model_guides_repair) has no Go equivalent
// (DECISIONS D-005) and is intentionally omitted; --schema-model is still
// accepted on the CLI surface so the mutual-exclusion checks keep parity.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertEqStr(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// captureOutput runs fn with os.Stdout/os.Stderr swapped for pipes and
// returns (stdout, stderr, exit code), mirroring pytest's capsys.
func captureOutput(t *testing.T, fn func() int) (string, string, int) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	code := fn()
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outB, _ := io.ReadAll(rOut)
	errB, _ := io.ReadAll(rErr)
	return string(outB), string(errB), code
}

// withStdin swaps os.Stdin with a temp file holding content for fn, mirroring
// pytest's patch("sys.stdin", ...).
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	f, err := os.CreateTemp("", "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	defer func() {
		os.Stdin = old
		f.Close()
	}()
	fn()
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLI(t *testing.T) {
	inPath := writeTemp(t, "in.json", "{key:value")
	outPath := writeTemp(t, "out.json", "")

	// stdout path with --indent 0 --ensure_ascii.
	out, stderr, code := captureOutput(t, func() int {
		return cli([]string{inPath, "--indent", "0", "--ensure_ascii"})
	})
	if code != 0 {
		t.Fatalf("cli: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, "{\n\"key\": \"value\"\n}\n")

	// -o writes the file without a trailing newline.
	out, stderr, code = captureOutput(t, func() int {
		return cli([]string{inPath, "--indent", "0", "-o", outPath})
	})
	if code != 0 {
		t.Fatalf("cli -o: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, "")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertEqStr(t, string(data), "{\n\"key\": \"value\"\n}")

	// -i rewrites the input file in place.
	_, stderr, code = captureOutput(t, func() int {
		return cli([]string{inPath, "--indent", "0", "-i"})
	})
	if code != 0 {
		t.Fatalf("cli -i: %s (code %d)", stderr, code)
	}
	data, err = os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	assertEqStr(t, string(data), "{\n\"key\": \"value\"\n}")

	// stdin path when no filename is given.
	withStdin(t, "{key:value", func() {
		out, stderr, code = captureOutput(t, func() int {
			return cli([]string{"--indent", "0"})
		})
		if code != 0 {
			t.Fatalf("cli stdin: %s (code %d)", stderr, code)
		}
		assertEqStr(t, out, "{\n\"key\": \"value\"\n}\n")
	})
}

func TestCLIInlineRequiresFilename(t *testing.T) {
	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{"--inline"})
	})
	assertEqStr(t, strings.TrimSpace(stderr), "Error: Inline mode requires a filename")
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLIInlineAndOutputError(t *testing.T) {
	outfile := writeTemp(t, "out.json", "")
	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{"dummy.json", "--inline", "--output", outfile})
	})
	assertEqStr(t, strings.TrimSpace(stderr), "Error: You cannot pass both --inline and --output")
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLISchemaFileGuidesRepair(t *testing.T) {
	schemaPath := writeTemp(t, "schema.json",
		`{"type": "object", "properties": {"value": {"type": "integer"}}, "required": ["value"]}`)
	inputPath := writeTemp(t, "input.json", `{"value": }`)

	out, stderr, code := captureOutput(t, func() int {
		return cli([]string{inputPath, "--indent", "0", "--schema", schemaPath})
	})
	if code != 0 {
		t.Fatalf("cli: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, "{\n\"value\": 0\n}\n")
}

func TestCLISchemaAppliesToValidJSON(t *testing.T) {
	schemaPath := writeTemp(t, "schema.json",
		`{"type": "object", "properties": {"value": {"type": "integer"}}, "required": ["value"]}`)
	inputPath := writeTemp(t, "input.json", `{"value": "1"}`)
	expected := "{\n\"value\": 1\n}\n"

	out, stderr, code := captureOutput(t, func() int {
		return cli([]string{inputPath, "--indent", "0", "--schema", schemaPath})
	})
	if code != 0 {
		t.Fatalf("cli: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, expected)

	out, stderr, code = captureOutput(t, func() int {
		return cli([]string{inputPath, "--indent", "0", "--schema", schemaPath, "--skip-json-loads"})
	})
	if code != 0 {
		t.Fatalf("cli --skip-json-loads: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, expected)
}

func TestCLISchemaAndStrictError(t *testing.T) {
	schemaPath := writeTemp(t, "schema.json", `{"type": "integer"}`)
	inputPath := writeTemp(t, "input.json", `{"value": }`)

	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{inputPath, "--schema", schemaPath, "--strict"})
	})
	if !strings.Contains(strings.ToLower(stderr), "schema") {
		t.Fatalf("stderr %q does not mention schema", stderr)
	}
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLISchemaAndSchemaModelMutuallyExclusive(t *testing.T) {
	schemaPath := writeTemp(t, "schema.json", `{"type": "integer"}`)

	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{"dummy.json", "--schema", schemaPath, "--schema-model", "schema_model:SchemaModel"})
	})
	if !strings.Contains(strings.ToLower(stderr), "schema") {
		t.Fatalf("stderr %q does not mention schema", stderr)
	}
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLISchemaModelNotSupported(t *testing.T) {
	// The Go port has no Pydantic equivalent (D-005); the flag is accepted on
	// the surface (so mutual-exclusion checks keep parity) but always fails.
	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{"dummy.json", "--schema-model", "schema_model:SchemaModel"})
	})
	if !strings.Contains(stderr, "--schema-model is not supported") {
		t.Fatalf("stderr %q does not mention unsupported --schema-model", stderr)
	}
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLISchemaRepairModeSalvageRequiresSchema(t *testing.T) {
	_, stderr, code := captureOutput(t, func() int {
		return cli([]string{"--schema-repair-mode", "salvage"})
	})
	if !strings.Contains(strings.ToLower(stderr), "schema-repair-mode") {
		t.Fatalf("stderr %q does not mention schema-repair-mode", stderr)
	}
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestCLISchemaRepairModeSalvageDropsInvalidItems(t *testing.T) {
	schemaPath := writeTemp(t, "schema.json",
		`{"type": "object", "properties": {"items": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}, "score": {"type": "number"}}, "required": ["id", "score"]}}}, "required": ["items"]}`)
	inputPath := writeTemp(t, "input.json",
		`{"items":[{"id":1,"score":85.6},{"id":2,"score":"N/A"}]}`)

	out, stderr, code := captureOutput(t, func() int {
		return cli([]string{inputPath, "--indent", "0", "--schema", schemaPath, "--schema-repair-mode", "salvage"})
	})
	if code != 0 {
		t.Fatalf("cli: %s (code %d)", stderr, code)
	}
	assertEqStr(t, out, "{\n\"items\": [\n{\n\"id\": 1,\n\"score\": 85.6\n}\n]\n}\n")
}
