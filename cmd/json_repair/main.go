// Command json_repair is the Go port of the Python `json_repair` CLI
// (../json_repair-main/src/json_repair/__main__.py + json_repair.py: cli()).
// It repairs and parses JSON from a file or stdin, writing the result to
// stdout, --output, or back to the input file with --inline.
package main

import "os"

func main() {
	os.Exit(cli(os.Args[1:]))
}
