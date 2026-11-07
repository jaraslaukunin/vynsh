package tui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/trash"
)

// TestWriteScreenshots renders full frames for the README.
// Run with: VYNSH_SHOT=/tmp/shot go test ./internal/tui -run TestWriteScreenshots
func TestWriteScreenshots(t *testing.T) {
	prefix := os.Getenv("VYNSH_SHOT")
	if prefix == "" {
		t.Skip("set VYNSH_SHOT=/path/prefix to write screenshots")
	}
	m := sampleModel()
	m.width, m.height = 100, 30
	m.version = "0.3 beta"
	m.frame = 12
	m.phase = phaseReady
	m.trashRep = &trash.Report{Items: 3, Bytes: 1.2e9}
	m.addLog("ok", "freed 8.72 GB in 59 targets")

	for tab, name := range map[tabIndex]string{tabOverview: "overview", tabCleanup: "cleanup"} {
		m.tab = tab
		out := m.View() + "\n"
		if err := os.WriteFile(prefix+name+".txt", []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		if testing.Verbose() {
			fmt.Printf("\n=== %s ===\n%s\n", name, out)
		}
		_ = time.Now
	}
}
