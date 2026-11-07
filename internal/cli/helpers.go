package cli

import (
	"encoding/json"
	"io"
	"os"
	"runtime"

	"github.com/jaraslaukunin/vynsh/internal/audit"
)

func osName() string { return runtime.GOOS + "/" + runtime.GOARCH }

func auditPath() string { return audit.Path() }

func newJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}

// isPipeAnswer reports whether stdin can be read for a yes/no answer even
// though it is not a terminal (e.g. "echo yes | vynsh clean").
func isPipeAnswer() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice == 0
}

// mustJSON writes v as JSON or returns the encoding error.
func mustJSON(v any) error { return newJSONEncoder(os.Stdout).Encode(v) }

// writeJSON is an alias kept for readability in commands.
func writeJSON(v any) error { return mustJSON(v) }
