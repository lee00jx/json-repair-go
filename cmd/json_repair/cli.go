package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	json_repair "github.com/lee00jx/json-repair-go"
)

// cliFlags is the parsed CLI state, mirroring Python cli()'s argparse dests.
type cliFlags struct {
	filename    string
	hasFilename bool
	inline      bool
	output      string
	ensureASCII bool
	indent      int // default 2
	skipLoads   bool
	schemaPath  string
	schemaModel string
	strict      bool
	mode        string // "standard" (default) | "salvage"
}

// parseCLIArgs mirrors Python cli()'s argparse setup: one optional positional
// filename, boolean store_true flags, and value flags (space or `=` form).
// It returns an error for usage mistakes; cli() reports them and exits 2,
// the same way argparse does.
func parseCLIArgs(args []string) (*cliFlags, error) {
	f := &cliFlags{indent: 2, mode: "standard"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		var inlineVal string
		hasInline := false
		if strings.HasPrefix(a, "--") {
			if eq := strings.IndexByte(a, '='); eq > 1 {
				name = a[:eq]
				inlineVal = a[eq+1:]
				hasInline = true
			}
		}
		takeValue := func() (string, error) {
			if hasInline {
				return inlineVal, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("argument %s: expected one argument", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "-i", "--inline":
			f.inline = true
		case "--ensure_ascii":
			f.ensureASCII = true
		case "--skip-json-loads":
			f.skipLoads = true
		case "--strict":
			f.strict = true
		case "-o", "--output":
			v, err := takeValue()
			if err != nil {
				return nil, err
			}
			f.output = v
		case "--indent":
			v, err := takeValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("argument --indent: invalid int value %q", v)
			}
			f.indent = n
		case "--schema":
			v, err := takeValue()
			if err != nil {
				return nil, err
			}
			f.schemaPath = v
		case "--schema-model":
			v, err := takeValue()
			if err != nil {
				return nil, err
			}
			f.schemaModel = v
		case "--schema-repair-mode":
			v, err := takeValue()
			if err != nil {
				return nil, err
			}
			if v != "standard" && v != "salvage" {
				return nil, fmt.Errorf("argument --schema-repair-mode: invalid choice: %q (choose from 'standard', 'salvage')", v)
			}
			f.mode = v
		default:
			if strings.HasPrefix(a, "-") && len(a) > 1 {
				return nil, fmt.Errorf("unrecognized argument: %s", a)
			}
			if f.hasFilename {
				return nil, fmt.Errorf("unrecognized argument: %s", a)
			}
			f.filename = a
			f.hasFilename = true
		}
	}
	return f, nil
}

// cli is the testable entry point mirroring Python cli(inline_args). It reads
// os.Stdin when no filename is given and writes through os.Stdout/os.Stderr,
// which tests may swap for capture. It returns the process exit code.
func cli(args []string) int {
	flags, err := parseCLIArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 2
	}

	// Pre-checks, in the same order as Python cli().
	if flags.inline && !flags.hasFilename {
		fmt.Fprintln(os.Stderr, "Error: Inline mode requires a filename")
		return 1
	}
	if flags.inline && flags.output != "" {
		fmt.Fprintln(os.Stderr, "Error: You cannot pass both --inline and --output")
		return 1
	}
	if flags.schemaPath != "" && flags.schemaModel != "" {
		fmt.Fprintln(os.Stderr, "Error: You cannot pass both --schema and --schema-model")
		return 1
	}
	if flags.strict && (flags.schemaPath != "" || flags.schemaModel != "") {
		fmt.Fprintln(os.Stderr, "Error: --strict cannot be used with --schema or --schema-model")
		return 1
	}
	if flags.mode == "salvage" && flags.schemaPath == "" && flags.schemaModel == "" {
		fmt.Fprintln(os.Stderr, "Error: --schema-repair-mode salvage requires --schema or --schema-model")
		return 1
	}
	if flags.schemaModel != "" {
		// Pydantic v2 models have no Go equivalent (DECISIONS D-005). The flag
		// is kept for CLI surface parity and always fails in the Go port.
		fmt.Fprintln(os.Stderr, "Error: --schema-model is not supported by the Go port; use --schema with a JSON Schema file")
		return 1
	}

	opts := json_repair.Options{
		ReturnObjects:    true,
		SkipJsonLoads:    flags.skipLoads,
		Strict:           flags.strict,
		SchemaRepairMode: flags.mode,
	}
	if flags.schemaPath != "" {
		schema, err := loadSchemaFile(flags.schemaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		opts.Schema = schema
	}

	// Use FromFile when a filename is provided; otherwise read all of stdin.
	var result any
	if flags.hasFilename {
		result, _, err = json_repair.FromFile(flags.filename, opts)
	} else {
		result, _, err = json_repair.Load(os.Stdin, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	dumped, err := json_repair.Dump(result, json_repair.DumpOptions{
		EnsureASCII: flags.ensureASCII,
		Pretty:      true,
		Indent:      flags.indent,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if flags.inline || flags.output != "" {
		target := flags.output
		if target == "" {
			target = flags.filename
		}
		if err := os.WriteFile(target, []byte(dumped), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintln(os.Stdout, dumped)
	}
	return 0
}

// loadSchemaFile mirrors Python cli()'s `json.load` on the --schema file:
// strict decoding (no repair) with the int/float type mapping (D-002), so a
// schema like {"minimum": 1} keeps integer 1 rather than 1.0.
func loadSchemaFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return json_repair.DecodeJSON(string(data))
}
