// Package json_repair is a Go port of the Python library `json_repair`
// (https://github.com/mangiucugna/json_repair). It repairs malformed JSON
// produced by LLMs, APIs, logs and user input, and is intended to act as a
// drop-in fallback for encoding/json.
//
// The port targets 1:1 behavioural parity with the Python original. The
// authoritative behavioural spec is the Python test suite in the sibling
// project (json_repair-main/tests); each Go test mirrors those cases.
//
// Physical layout: the whole parser lives in a single package (package
// json_repair) because the repair heuristics heavily cross-reference
// unexported parser state (the token cursor, the object-member context,
// lookahead scans). The package is organised by logical module, documented
// under .pmemory/modules:
//
//   - api            RepairJSON / Loads / Load / FromFile entry points (json_repair.go)
//   - parser         JSONParser core: cursor, context, dispatch, top-level
//     merging, strict / stream_stable (json_parser.go)
//   - parse_string   string parsing, the most heuristic-heavy path (parse_string.go)
//   - containers     object / array / number / comment / tuple (parse_*.go,
//     parser_parenthesized.go)
//   - schema_repair  SchemaRepairer, standard + salvage modes, patternProperties
//     (schema_repair.go, parser_schema.go)
//   - utils          constants, JsonContext, ObjectComparer, StringFileWrapper
//     (constants.go, json_context.go, object_comparer.go,
//     string_file_wrapper.go, pattern_properties.go)
//   - cli            command-line entry point (cmd/json_repair)
package json_repair
