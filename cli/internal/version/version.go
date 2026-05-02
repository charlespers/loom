// Package version exposes the CLI version constants and the body of the
// `loom version` subcommand.
package version

import (
	"fmt"
	"io"
)

// CLIVersion is the user-facing CLI version. Bumped via release tags.
const CLIVersion = "0.1.0"

// WireSchema is the wire format version this CLI knows how to read.
const WireSchema = "loom.event.v1"

// Print writes a multi-line version block to w.
func Print(w io.Writer) {
	fmt.Fprintf(w, "loom         %s\n", CLIVersion)
	fmt.Fprintf(w, "wire schema  %s\n", WireSchema)
	fmt.Fprintf(w, "daemon       (run `loom doctor` to verify discoverable)\n")
}
