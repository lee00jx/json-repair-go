package json_repair

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Options mirrors the flags of the Python repair_json function.
type Options struct {
	ReturnObjects    bool
	SkipJsonLoads    bool
	Logging          bool
	StreamStable     bool
	Strict           bool
	Schema           any
	SchemaRepairMode string
	// json.dumps passthrough (used by the string-output helpers).
	// EnsureASCII mirrors json.dumps(ensure_ascii=...); for the string-output
	// path the effective default is true (Python repair_json default).
	EnsureASCII bool
	Pretty      bool
	Indent      int
}

// RepairJSON repairs a JSON string and returns the decoded object (the
// Python return_objects=True form). It first tries the standard library
// parser (unless SkipJsonLoads) and falls back to the repair parser.
//
// When Schema is set, both valid and invalid JSON go through schema-guided
// repair + validation (Python invariant). Schema and Strict are mutually
// exclusive; schema_repair_mode="salvage" requires Schema.
//
// The returned log is empty unless opts.Logging is true.
func RepairJSON(jsonStr string, opts Options) (any, []RepairLog, error) {
	var modePtr *string
	if opts.SchemaRepairMode != "" {
		modePtr = &opts.SchemaRepairMode
	}
	mode, err := normalizeSchemaRepairMode(modePtr)
	if err != nil {
		return nil, nil, err
	}
	if opts.Schema == nil && mode == "salvage" {
		return nil, nil, fmt.Errorf("schema_repair_mode='salvage' requires schema.")
	}
	if opts.Schema != nil && opts.Strict {
		return nil, nil, fmt.Errorf("schema and strict cannot be used together.")
	}

	var schemaObj any
	if opts.Schema != nil {
		schemaObj, err = SchemaFromInput(opts.Schema)
		if err != nil {
			return nil, nil, err
		}
	}

	// Shared log slice so parser and SchemaRepairer append in chronological order.
	var logs []RepairLog
	var logPtr *[]RepairLog
	if opts.Logging {
		logPtr = &logs
	}

	var repairer *SchemaRepairer
	if schemaObj != nil {
		repairer, err = NewSchemaRepairer(schemaObj, logPtr, &mode)
		if err != nil {
			return nil, nil, err
		}
	}

	parsed := any(nil)
	isValidJSON := false
	if !opts.SkipJsonLoads {
		if v, ok := stdlibDecode(jsonStr); ok {
			if repairer != nil && schemaObj != nil {
				valid, verr := repairer.isValid(v, schemaObj)
				if verr != nil {
					return nil, logs, verr
				}
				if valid {
					parsed = v
					isValidJSON = true
				} else {
					repaired, rerr := repairer.RepairValue(v, schemaObj, "$")
					if rerr == nil {
						valid2, verr2 := repairer.isValid(repaired, schemaObj)
						if verr2 != nil {
							return nil, logs, verr2
						}
						if valid2 {
							parsed = repaired
							isValidJSON = true
						}
					}
				}
			} else {
				parsed = v
				isValidJSON = true
			}
		}
	}

	if !isValidJSON {
		p := newJSONParser(jsonStr, opts.Logging, opts.StreamStable, opts.Strict, !opts.SkipJsonLoads)
		if opts.Logging {
			p.logger = logs
			if repairer != nil {
				repairer.log = &p.logger
			}
		}
		if repairer != nil && schemaObj != nil {
			parsed = p.ParseWithSchema(repairer, schemaObj)
			if p.schemaErr != nil {
				return parsed, p.logger, p.schemaErr
			}
			if verr := repairer.validate(parsed, schemaObj); verr != nil {
				return parsed, p.logger, verr
			}
		} else {
			parsed = p.Parse()
			if p.strictErr != nil {
				return parsed, p.logger, p.strictErr
			}
		}
		if opts.Logging {
			logs = p.logger
		}
	}
	return parsed, logs, nil
}

// Loads is the Python loads() equivalent: repair and return the object.
func Loads(jsonStr string, opts Options) (any, []RepairLog, error) {
	opts.ReturnObjects = true
	return RepairJSON(jsonStr, opts)
}

// RepairJSONString repairs a JSON string and returns the serialized JSON
// text, mirroring repair_json(..., return_objects=False). It runs the same
// repair as RepairJSON and then serializes the result with Dump.
//
// The empty-string special case from Python is preserved: a repair that
// yields an empty string returns "" rather than `""`.
//
// EnsureASCII/Pretty/Indent are taken from opts as-is (Python repair_json's
// string path defaults to ensure_ascii=True, compact; the CLI defaults to
// ensure_ascii=False, indent=2 — set opts accordingly per call site).
func RepairJSONString(jsonStr string, opts Options) (string, []RepairLog, error) {
	v, logs, err := RepairJSON(jsonStr, opts)
	if err != nil {
		return "", logs, err
	}
	if v == "" {
		return "", logs, nil
	}
	s, err := Dump(v, DumpOptions{
		EnsureASCII: opts.EnsureASCII,
		Pretty:      opts.Pretty,
		Indent:      opts.Indent,
	})
	return s, logs, err
}

// Load reads and repairs JSON from a reader (Python load()). For stage 1a the
// whole reader is buffered into a string; the chunked StringFileWrapper is
// deferred to stage 3.
func Load(fd io.Reader, opts Options) (any, []RepairLog, error) {
	data, err := io.ReadAll(fd)
	if err != nil {
		return nil, nil, err
	}
	opts.ReturnObjects = true
	return RepairJSON(string(data), opts)
}

// FromFile reads and repairs JSON from a file (Python from_file()).
func FromFile(filename string, opts Options) (any, []RepairLog, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}
	opts.ReturnObjects = true
	return RepairJSON(string(data), opts)
}

// DecodeJSON parses a whole string as a single strict JSON value with the
// library's type mapping (DECISIONS D-002): integers as int64, decimals as
// float64. Object keys keep document (insertion) order in the returned
// *OrderedMap, mirroring Python json.loads (an ordered dict). It is used by
// the CLI to load schema files (Python cli() uses json.load there, not the
// repairer).
func DecodeJSON(s string) (any, error) {
	v, _, err := decodeOrderedStrict(s)
	return v, err
}

// stdlibDecode decodes the entire input as strict JSON using the standard
// library, converting integers to int64 and numbers to float64 to match the
// Python int/float distinction (DECISIONS D-002). Object keys keep document
// order (CPython json builds dicts in insertion order). It reports ok=false
// unless the whole input is exactly one JSON value (plus trailing whitespace).
func stdlibDecode(s string) (any, bool) {
	v, _, err := decodeOrderedStrict(s)
	if err != nil {
		return nil, false
	}
	return v, true
}

// decodeOrderedStrict strictly decodes the entire input as a single JSON
// value over the standard library token stream, preserving object key
// insertion order (DECISIONS D-006) and applying the D-002 number mapping
// (int64 / float64). It returns the number of bytes consumed by the value —
// exactly the end of the value, like Python json.JSONDecoder.raw_decode —
// and an error unless the input is exactly one JSON value plus optional
// trailing whitespace.
func decodeOrderedStrict(s string) (any, int, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, 0, err
	}
	bytesConsumed := int(dec.InputOffset())
	// json.loads rejects trailing non-whitespace; ensure the stream is done.
	if _, err := dec.Token(); err != io.EOF {
		return nil, 0, fmt.Errorf("trailing data after JSON value")
	}
	return v, bytesConsumed, nil
}

// decodeOrderedValue decodes one JSON value (including nested containers)
// from the token stream, building *OrderedMap for objects so key order
// survives.
func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			m := NewOrderedMap()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				val, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(key, val)
			}
			if _, err := dec.Token(); err != nil { // consume '}'
				return nil, err
			}
			return m, nil
		case '[':
			arr := []any{}
			for dec.More() {
				val, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil { // consume ']'
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected JSON delimiter %q", t)
	case string:
		return t, nil
	case bool:
		return t, nil
	case nil:
		return nil, nil
	case json.Number:
		// D-002: integer literals -> int64, everything else -> float64.
		if i, err := t.Int64(); err == nil {
			return i, nil
		}
		if f, err := t.Float64(); err == nil {
			return f, nil
		}
		return string(t), nil
	default:
		return nil, fmt.Errorf("unexpected JSON token %T", tok)
	}
}
