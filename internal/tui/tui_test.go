package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
)

func testModel() *model {
	return &model{
		mon:     sysinfo.NewMonitor(sysinfo.Options{Interval: 10 * time.Millisecond, TopN: 3}),
		refresh: time.Second,
		width:   120,
		height:  40,
	}
}

func sampleSnap() *sysinfo.Snapshot {
	return &sysinfo.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Platform: "macOS 1.0",
		Uptime:   3600,
		CPU: sysinfo.CPUInfo{
			Model: "Test CPU", Cores: 8, Percent: 12.5,
			PerCore: []float64{10, 15}, Load: sysinfo.Load{L1: 1, L5: 2, L15: 3},
		},
		Mem: sysinfo.MemInfo{
			Total: 16e9, Used: 8e9, Available: 8e9, Percent: 50,
			SwapTotal: 2e9, SwapUsed: 1e9,
		},
		Disks:     []sysinfo.DiskInfo{{Mountpoint: "/", Total: 500e9, Used: 300e9, Free: 200e9, Percent: 60}},
		Net:       []sysinfo.NetInfo{{Name: "en0", SendRate: 1000, RecvRate: 2000}},
		Procs:     []sysinfo.Proc{{PID: 42, Name: "testproc", CPU: 5, RSS: 1e8, Status: "R"}},
		ProcCount: 10,
	}
}

func TestViewRendersOverview(t *testing.T) {
	m := testModel()
	if got := m.View(); !strings.Contains(got, "vynsh") {
		t.Fatalf("empty view should still show the header, got %q", got)
	}
	m.Update(snapMsg{snap: sampleSnap()})
	out := m.View()
	for _, want := range []string{"CPU", "Memory", "Disks", "Network", "Top processes", "testproc"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview is missing %q:\n%s", want, out)
		}
	}
}

func TestTabSwitching(t *testing.T) {
	m := testModel()
	m.Update(snapMsg{snap: sampleSnap()})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.tab != 1 {
		t.Fatalf("tab = %d, want 1", m.tab)
	}
	if out := m.View(); !strings.Contains(out, "No scan yet") {
		t.Errorf("disk view should invite a scan:\n%s", out)
	}

	m.scanRes = &scan.Result{
		Root:  "/home/test",
		Total: 1000,
		Files: 10,
		TopDirs: []*scan.Node{
			{Name: "big", Path: "/home/test/big", Size: 900, IsDir: true},
		},
		TopFiles: []scan.FileEntry{{Path: "/home/test/big/blob", Size: 800}},
	}
	out := m.View()
	if !strings.Contains(out, "big") || !strings.Contains(out, "Largest files") {
		t.Errorf("disk view missing scan data:\n%s", out)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.tab != 2 {
		t.Fatalf("tab = %d, want 2", m.tab)
	}
	m.junk = []clean.Target{{
		Rule:   clean.Rule{ID: "user-caches", Name: "All user app caches", Category: "apps", Risk: clean.RiskLow},
		Path:   "/home/test/Library/Caches/x",
		Size:   1234,
		Exists: true,
	}}
	m.junkSize = 1234
	if out := m.View(); !strings.Contains(out, "reclaimable") {
		t.Errorf("junk view missing header:\n%s", out)
	}
}

func TestQuitKey(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("quit key must return a command")
	}
	if !m.quitting {
		t.Error("model should be marked as quitting")
	}
	if m.View() != "" {
		t.Error("quitting model must render nothing")
	}
}
