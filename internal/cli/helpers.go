package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/style"
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

// mayDelete decides whether a destructive action should run.
//
//   - --dry-run always wins: measure only.
//   - --yes deletes without asking.
//   - at a terminal vynsh shows the plan and asks.
//   - without a terminal (scripts, cron) it refuses and explains why, so an
//     unattended run can never delete by surprise.
//
// The returned hint is meant to be printed when mayDelete returns false.
func mayDelete(out io.Writer, question, dryHint string) bool {
	if g.yes {
		return true
	}
	if style.IsTerminal(os.Stdin) {
		if confirm(question) {
			return true
		}
		fmt.Fprintln(out, g.th.Dim("aborted — nothing was changed"))
		return false
	}
	fmt.Fprintln(out, g.th.Dim(dryHint))
	return false
}
