package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func sampleSnapshot() *sysinfo.Snapshot {
	return &sysinfo.Snapshot{
		Time:     time.Now(),
		Hostname: "macbook",
		Platform: "macOS 27.0",
		Uptime:   3 * 24 * 3600,
		CPU: sysinfo.CPUInfo{Model: "Apple M2", Cores: 8, Percent: 12.4,
			Load: sysinfo.Load{L1: 1.2, L5: 1.5, L15: 1.7}},
		Mem:       sysinfo.MemInfo{Total: 16e9, Used: 9.8e9, Available: 6.2e9, Percent: 61, SwapTotal: 2e9, SwapUsed: 0.5e9},
		Disks:     []sysinfo.DiskInfo{{Mountpoint: "/", Total: 494e9, Used: 380e9, Free: 114e9, Percent: 77}},
		Net:       []sysinfo.NetInfo{{Name: "en0", SendRate: 300e3, RecvRate: 1.2e6}},
		Procs:     []sysinfo.Proc{{PID: 4711, Name: "WindowServer", CPU: 42.1, RSS: 512e6}},
		ProcCount: 412,
	}
}

func sampleModel() *model {
	m := newModel(Options{})
	m.width, m.height = 110, 34
	m.snap = sampleSnapshot()
	m.rules = []ruleRow{
		{rule: clean.Rule{ID: "user-caches", Name: "All user app caches", Category: "apps", Risk: clean.RiskLow}, size: 8.72e9, paths: 59},
		{rule: clean.Rule{ID: "gradle-cache", Name: "Gradle caches", Category: "dev", Risk: clean.RiskMedium}, size: 3.29e9, paths: 1},
		{rule: clean.Rule{ID: "go-build-cache", Name: "Go build cache", Category: "dev", Risk: clean.RiskLow}, size: 238e6, paths: 1},
		{rule: clean.Rule{ID: "xcode-archives", Name: "Xcode archives", Category: "dev", Risk: clean.RiskHigh}, size: 0, paths: 0},
	}
	m.selected = map[string]bool{"user-caches": true, "go-build-cache": true}
	m.phase = phaseReady
	return m
}

// TestRenderAllViews prints every view so the layout can be reviewed, and
// asserts that the important pieces are present.
func TestRenderAllViews(t *testing.T) {
	m := sampleModel()
	m.trashRep = &trash.Report{Items: 3, Bytes: 1.2e9, Locations: []trash.Info{{Path: "/home/u/.Trash", Items: 3, Bytes: 1.2e9}}}
	m.restorable = []trash.Record{{Original: "/home/u/Downloads/old.dmg", Bytes: 900e6, Time: time.Now().Add(-2 * time.Hour)}}

	checks := map[tabIndex][]string{
		tabOverview: {"CPU", "Memory", "Disks", "Network", "Top processes", "WindowServer", "vynsh"},
		tabCleanup:  {"Cleanup", "All user app caches", "Gradle caches", "selected"},
		tabDisk:     {"No scan yet", "press", "analyse"},
		tabTrash:    {"Trash", "Restorable by vynsh", "old.dmg"},
	}
	for tab, wants := range checks {
		m.tab = tab
		out := m.View()
		if testing.Verbose() {
			fmt.Printf("\n=== %s ===\n%s\n", tabNames[tab], out)
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("%s view is missing %q", tabNames[tab], want)
			}
		}
		if got, limit := lipgloss.Width(out), m.width+4; got > limit {
			t.Errorf("%s view is %d columns wide, terminal is %d", tabNames[tab], got, m.width)
		}
	}
}

func TestCleanupSelectionAndPrompt(t *testing.T) {
	m := sampleModel()
	m.tab = tabCleanup
	if got := m.selectedSize(); got != 8.72e9+238e6 {
		t.Fatalf("selected size = %v", got)
	}
	// Toggle the first row off.
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.selected["user-caches"] {
		t.Error("space should unselect the cursor rule")
	}
	// "a" selects everything safe again.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.selected["user-caches"] || !m.selected["gradle-cache"] {
		t.Error("'a' should select low and medium rules")
	}
	if m.selected["xcode-archives"] {
		t.Error("'a' must not select high risk rules")
	}
	// Enter asks for confirmation instead of deleting right away.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil {
		t.Fatal("enter should open a confirmation prompt")
	}
	if !strings.Contains(m.View(), "[y/N]") {
		t.Error("prompt is not rendered in the footer")
	}
	// "n" cancels.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.prompt != nil {
		t.Error("prompt should be cancelled by 'n'")
	}
}

func TestQuit(t *testing.T) {
	m := sampleModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || !m.quitting {
		t.Fatal("q must quit")
	}
	if m.View() != "" {
		t.Error("quitting model should render nothing")
	}
}

func TestTabSwitching(t *testing.T) {
	m := sampleModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.tab != tabCleanup {
		t.Fatalf("tab = %v, want cleanup", m.tab)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.tab != tabDisk {
		t.Fatalf("tab = %v, want disk", m.tab)
	}
}

func TestDiskViewWithScan(t *testing.T) {
	m := sampleModel()
	m.tab = tabDisk
	m.scanRes = &scan.Result{
		Root: "/home/u", Total: 100e9, Files: 1000,
		TopDirs:  []*scan.Node{{Name: "Library", Path: "/home/u/Library", Size: 87e9, IsDir: true}},
		TopFiles: []scan.FileEntry{{Path: "/home/u/Library/x.dmg", Size: 40e9}},
	}
	out := m.View()
	for _, want := range []string{"Disk usage", "Library", "Largest files", "x.dmg"} {
		if !strings.Contains(out, want) {
			t.Errorf("disk view missing %q", want)
		}
	}
	_ = units.HumanBytes
}

func TestMascotFrames(t *testing.T) {
	for i := 0; i < len(mascotFrames)+2; i++ {
		out := mascotPanel(i, "measuring caches…", true, 26)
		if !strings.Contains(out, "╭") || (!strings.Contains(out, "▓") && !strings.Contains(out, "▀")) {
			t.Errorf("frame %d looks broken:\n%s", i, out)
		}
	}
}

func TestNarrowTerminal(t *testing.T) {
	m := sampleModel()
	m.width, m.height = 80, 24
	m.trashRep = &trash.Report{Items: 2, Bytes: 3e8}
	for tab := tabIndex(0); tab < tabCount; tab++ {
		m.tab = tab
		out := m.View()
		if testing.Verbose() {
			fmt.Printf("\n=== narrow %s ===\n%s\n", tabNames[tab], out)
		}
		for _, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > m.width+2 {
				t.Errorf("%s: line is %d wide in an %d column terminal: %q",
					tabNames[tab], lipgloss.Width(line), m.width, line)
			}
		}
	}
}
