package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/logo"
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
		tabDisk:     {"No scan yet", "Press", "analyse"},
		tabDupes:    {"Duplicates", "Find files with identical content"},
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
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	if m.tab != tabDupes {
		t.Fatalf("key 4 should open duplicates, got %v", m.tab)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.tab != tabTrash {
		t.Fatalf("key 5 should open the trash, got %v", m.tab)
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
	if err := logo.Check(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 90; i++ {
		m := sampleModel()
		m.frame = i
		m.height = 34
		out := m.withBeetle("BOX", 110)
		if !strings.Contains(out, "█") {
			t.Errorf("frame %d has no beetle pixels:\n%s", i, out)
		}
		if !strings.Contains(out, "BOX") {
			t.Errorf("frame %d lost the input box", i)
		}
	}
}

func TestBeetleHops(t *testing.T) {
	// While hopping the beetle lifts off: its art gains an empty first row and
	// drops the feet, so the figure sits one row higher.
	var lifted, grounded int
	for i := 0; i < 30; i++ {
		m := sampleModel()
		m.frame = i
		m.height = 34
		out := m.withBeetle("BOX", 110)
		first := strings.Split(out, "\n")[0]
		if strings.TrimSpace(first) == "" {
			lifted++
		} else {
			grounded++
		}
	}
	if lifted == 0 || grounded == 0 {
		t.Errorf("beetle never changed pose: lifted=%d grounded=%d", lifted, grounded)
	}
	if !(beetleState{frame: 6}).hopping() {
		t.Error("frame 6 should be airborne")
	}
	if (beetleState{frame: 20}).hopping() {
		t.Error("frame 20 should be on the ground")
	}
	// Every animation state must produce a distinct sprite at some point.
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		st := beetleState{frame: i}
		seen[spriteName(st.sprite())] = true
	}
	for _, want := range []string{"base", "hop", "blink", "wink", "antennae"} {
		if !seen[want] {
			t.Errorf("idle animation never showed %q (seen: %v)", want, seen)
		}
	}
	busy := map[string]bool{}
	for i := 0; i < 20; i++ {
		busy[spriteName(beetleState{frame: i, busy: true}.sprite())] = true
	}
	if !busy["type1"] && !busy["type2"] {
		t.Errorf("busy state does not animate the feet: %v", busy)
	}
	// Celebration is a double hop rather than a squint, so the eyes stay solid.
	hops := 0
	for i := 0; i < 24; i++ {
		if (beetleState{frame: i, happy: true}).hopping() {
			hops++
		}
	}
	if hops < 4 {
		t.Errorf("celebration should hop twice, saw %d airborne frames", hops)
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

// spriteName finds the registry name of a sprite for readable assertions.
func spriteName(s logo.Sprite) string {
	for name, candidate := range logo.All() {
		if len(candidate) != len(s) {
			continue
		}
		same := true
		for i := range candidate {
			if candidate[i] != s[i] {
				same = false
				break
			}
		}
		if same {
			return name
		}
	}
	return "custom"
}

func TestBeetleSitsInTheRightCorner(t *testing.T) {
	m := sampleModel()
	m.height = 34
	width := 110
	out := m.withBeetle("BOX", width)
	var artLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "█") {
			artLine = line
			break
		}
	}
	if artLine == "" {
		t.Fatal("no beetle art found")
	}
	start := lipgloss.Width(artLine) - lipgloss.Width(strings.TrimLeft(artLine, " "))
	if start < width/2 {
		t.Errorf("beetle starts at column %d of %d: it should sit on the right", start, width)
	}
	if end := start + logo.Base.Width()*2; end > width-2 {
		t.Errorf("beetle ends at %d, past the box edge %d", end, width-2)
	}
}

func TestBeetleWalksOnlyALittle(t *testing.T) {
	m := sampleModel()
	m.height = 34
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		m.frame = i
		m.stepBeetle()
		seen[m.beetleShift] = true
		if m.beetleShift > 0 || m.beetleShift < -4 {
			t.Fatalf("beetle wandered to %d columns", m.beetleShift)
		}
	}
	if len(seen) < 2 {
		t.Errorf("beetle never moved: %v", seen)
	}
}
