package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// contentWidth is the horizontal space available for the active view.
func (m *model) contentWidth() int {
	w := m.width
	if w < 40 {
		w = 100
	}
	if w >= 88 {
		return w - 35
	}
	return w - 2
}

// section renders a small heading with a rule underneath.
func section(title, right string, width int) string {
	head := titleStyle.Render(title)
	if right != "" {
		gap := width - lipgloss.Width(head) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}
		head += repeat(" ", gap) + right
	}
	return head + "\n" + dimStyle.Render(repeat("─", max0(width)))
}

// riskBadge renders the risk level with its colour.
func riskBadge(r clean.Risk) string {
	switch r {
	case clean.RiskHigh:
		return badStyle.Render("high  ")
	case clean.RiskMedium:
		return warnStyle.Render("medium")
	default:
		return goodStyle.Render("low   ")
	}
}

// ---- overview -----------------------------------------------------------

func (m *model) overviewView() string {
	width := m.contentWidth()
	if m.snap == nil {
		return section("Overview", "", width) + "\n" + dimStyle.Render("collecting the first sample…")
	}
	s := m.snap
	half := width / 2
	cw := half - 6
	if cw < 18 {
		cw = 18
	}

	cpuHead := clamp(fmt.Sprintf("%s · %d cores · %s", s.CPU.Model, s.CPU.Cores,
		valueStyle.Render(units.Percent(s.CPU.Percent, 1))), cw)
	cpuLoad := "load —"
	if s.CPU.Load.L1 > 0 {
		cpuLoad = fmt.Sprintf("load %.2f %.2f %.2f", s.CPU.Load.L1, s.CPU.Load.L5, s.CPU.Load.L15)
	}
	cpuBox := boxStyle.Width(cw + 2).Render(strings.Join([]string{
		titleStyle.Render("CPU"),
		cpuHead,
		clamp(dimStyle.Render(cpuLoad), cw),
		bar(s.CPU.Percent, cw),
	}, "\n"))

	memHead := clamp(fmt.Sprintf("%s / %s · %s",
		valueStyle.Render(units.HumanBytes(int64(s.Mem.Used))),
		units.HumanBytes(int64(s.Mem.Total)), units.Percent(s.Mem.Percent, 1)), cw)
	memSwap := "swap not used"
	if s.Mem.SwapTotal > 0 {
		memSwap = fmt.Sprintf("swap %s / %s",
			units.HumanBytes(int64(s.Mem.SwapUsed)), units.HumanBytes(int64(s.Mem.SwapTotal)))
	}
	memBox := boxStyle.Width(cw + 2).Render(strings.Join([]string{
		titleStyle.Render("Memory"),
		memHead,
		clamp(labelStyle.Render(memSwap), cw),
		bar(s.Mem.Percent, cw),
	}, "\n"))

	var b strings.Builder
	b.WriteString(section("Overview",
		labelStyle.Render("up "+units.HumanDuration(time.Duration(s.Uptime)*time.Second)), width))
	b.WriteString("\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cpuBox, memBox))
	b.WriteString("\n")

	fullW := cw*2 + 4
	if len(s.Disks) > 0 {
		rows := make([]string, 0, len(s.Disks))
		for _, d := range s.Disks {
			rows = append(rows, clamp(fmt.Sprintf("%s %s %s %s",
				labelStyle.Render(padRight(d.Mountpoint, 18)), bar(d.Percent, 18),
				valueStyle.Render(units.HumanBytes(int64(d.Free))),
				dimStyle.Render("free of "+units.HumanBytes(int64(d.Total)))), fullW))
		}
		b.WriteString(boxStyle.Width(fullW).Render(titleStyle.Render("Disks") + "\n" + strings.Join(rows, "\n")))
		b.WriteString("\n")
	}

	var netRows []string
	for _, n := range s.Net {
		if n.SendRate == 0 && n.RecvRate == 0 {
			continue
		}
		netRows = append(netRows, clamp(fmt.Sprintf("%s ↓ %s ↑ %s",
			padRight(n.Name, 12), units.HumanRate(n.RecvRate), units.HumanRate(n.SendRate)), fullW))
	}
	if len(netRows) > 0 {
		b.WriteString(boxStyle.Width(fullW).Render(titleStyle.Render("Network") + "\n" + strings.Join(netRows, "\n")))
		b.WriteString("\n")
	}

	if len(s.Procs) > 0 {
		procRows := make([]string, 0, len(s.Procs))
		for _, p := range s.Procs {
			procRows = append(procRows, clamp(fmt.Sprintf("%s %s %s %s",
				labelStyle.Render(padLeft(units.HumanCount(int64(p.PID)), 7)),
				padRight(units.Percent(p.CPU, 1), 7),
				padRight(units.HumanBytes(int64(p.RSS)), 9),
				textStyle.Render(p.Name)), fullW))
		}
		b.WriteString(boxStyle.Width(fullW).Render(
			titleStyle.Render(fmt.Sprintf("Top processes · %d running", s.ProcCount)) + "\n" +
				strings.Join(procRows, "\n")))
	}
	return b.String()
}

// ---- cleanup ------------------------------------------------------------

func (m *model) cleanupView() string {
	width := m.contentWidth()
	selected := fmt.Sprintf("%s selected · %s", units.HumanBytes(m.selectedSize()),
		plural(int64(m.selectedCount()), "rule", "rules"))
	head := section("Cleanup", labelStyle.Render(selected), width)

	if m.phase == phaseMeasuring && !m.hasSizes() {
		return head + "\n" + accentStyle.Render(spinner(m.frame)+" measuring caches…") + "\n" +
			dimStyle.Render(clamp("walking "+shorten(m.progress.path), width))
	}
	if !m.hasSizes() {
		return head + "\n" + dimStyle.Render("nothing measured yet — press r to measure again")
	}

	// Column widths adapt to the terminal.
	showID := width >= 96
	showPaths := width >= 72
	var b strings.Builder
	b.WriteString(head + "\n")

	listHeight := m.height - 14
	if listHeight < 5 {
		listHeight = 5
	}
	start := 0
	if m.cursor >= listHeight {
		start = m.cursor - listHeight + 1
	}
	end := start + listHeight
	if end > len(m.rules) {
		end = len(m.rules)
	}

	for i := start; i < end; i++ {
		r := m.rules[i]
		isCursor := i == m.cursor
		mark := "  "
		if isCursor {
			mark = cursorMark.Render("▸ ")
		}
		box := checkboxOff.Render("[ ]")
		if m.selected[r.rule.ID] {
			box = checkboxOn.Render("[x]")
		}
		size := padLeft(units.HumanBytes(r.size), 9)
		if r.size > 0 {
			size = colorBySize(r.size, m.selectedSize()) + ""
			size = padLeft(size, 9)
		} else {
			size = labelStyle.Render(padLeft("—", 9))
		}
		line := fmt.Sprintf("%s%s %s  %s ", mark, box, size, riskBadge(r.rule.Risk))
		if showID {
			line += dimStyle.Render(padRight(r.rule.ID, 22))
		}
		nameWidth := width - lipgloss.Width(line) - 6
		if showPaths {
			nameWidth -= 6
		}
		if nameWidth < 8 {
			nameWidth = 8
		}
		name := clamp(r.rule.Name, nameWidth)
		if isCursor {
			name = valueStyle.Render(name)
		} else {
			name = textStyle.Render(name)
		}
		line += name
		if showPaths {
			paths := ""
			if r.paths > 0 {
				paths = fmt.Sprintf("%d", r.paths)
			}
			line += repeat(" ", max0(width-lipgloss.Width(line)-len(paths))) + labelStyle.Render(paths)
		}
		b.WriteString(line + "\n")
	}
	if len(m.rules) > listHeight {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %d of %d rules shown — ↑↓ to scroll", listHeight, len(m.rules))) + "\n")
	}

	if m.applyRep != nil {
		b.WriteString("\n" + goodStyle.Render("✓ ") + textStyle.Render(fmt.Sprintf("last run freed %s",
			units.HumanBytes(m.applyRep.Bytes))))
		if len(m.applyRep.Failed) > 0 {
			b.WriteString(warnStyle.Render(fmt.Sprintf(" · %d targets failed", len(m.applyRep.Failed))))
		}
	}
	return b.String()
}

// colorBySize highlights the biggest contributors.
func colorBySize(size, total int64) string {
	text := units.HumanBytes(size)
	if total <= 0 {
		return textStyle.Render(text)
	}
	switch share := float64(size) / float64(total); {
	case share >= 0.5:
		return warnStyle.Render(text)
	case size >= 1_000_000_000:
		return accentStyle.Render(text)
	}
	return textStyle.Render(text)
}

func (m *model) hasSizes() bool {
	for _, r := range m.rules {
		if r.size > 0 {
			return true
		}
	}
	return false
}

// ---- disk ---------------------------------------------------------------

func (m *model) diskView() string {
	width := m.contentWidth()
	if m.scanRes == nil {
		head := section("Disk usage", "", width)
		lines := []string{
			dimStyle.Render("No scan yet."),
			"",
			textStyle.Render("Press ") + accentStyle.Render("s") + textStyle.Render(" to analyse your home directory."),
			labelStyle.Render("Deep scans of a large disk can take a while; the mascot will keep you company."),
		}
		if m.scanning {
			lines = append(lines, "", accentStyle.Render(spinner(m.frame)+" scanning "+shorten(fsutil.HomeDir())+"…"))
		}
		return head + "\n" + strings.Join(lines, "\n")
	}
	r := m.scanRes
	head := section("Disk usage", labelStyle.Render(fmt.Sprintf("%s · %s files",
		units.HumanBytes(r.Total), units.HumanCount(r.Files))), width)
	var b strings.Builder
	b.WriteString(head + "\n")
	b.WriteString(dimStyle.Render(padRight("SIZE", 10)+" "+padRight("SHARE", 6)+"  "+"PATH") + "\n")

	top := make([]*scan.Node, 0, len(r.TopDirs))
	top = append(top, r.TopDirs...)
	sort.Slice(top, func(i, j int) bool { return top[i].Size > top[j].Size })
	limit := m.height - 16
	if limit < 5 {
		limit = 5
	}
	if len(top) > limit {
		top = top[:limit]
	}
	for _, n := range top {
		pct := 0.0
		if r.Total > 0 {
			pct = float64(n.Size) / float64(r.Total) * 100
		}
		line := fmt.Sprintf("%s %s %s %s",
			padLeft(units.HumanBytes(n.Size), 10),
			labelStyle.Render(padLeft(units.Percent(pct, 1), 6)),
			bar(pct, 10),
			clamp(shorten(n.Path), width-30))
		b.WriteString(line + "\n")
	}
	if len(r.TopFiles) > 0 {
		b.WriteString("\n" + titleStyle.Render("Largest files") + "\n")
		for _, f := range r.TopFiles {
			b.WriteString(fmt.Sprintf("%s %s\n",
				padLeft(units.HumanBytes(f.Size), 10),
				clamp(shorten(f.Path), width-14)))
		}
	}
	b.WriteString("\n" + labelStyle.Render("v saves this scan as a snapshot — compare later with `vynsh snapshot diff`"))
	return b.String()
}

// ---- trash --------------------------------------------------------------

func (m *model) trashView() string {
	width := m.contentWidth()
	if m.trashRep == nil {
		return section("Trash", "", width) + "\n" + dimStyle.Render("reading the trash…")
	}
	right := labelStyle.Render(fmt.Sprintf("%s in %s",
		units.HumanBytes(m.trashRep.Bytes), plural(m.trashRep.Items, "item", "items")))
	var b strings.Builder
	b.WriteString(section("Trash", right, width) + "\n")

	permBlocked := false
	for _, loc := range m.trashRep.Locations {
		if loc.Err == "" {
			continue
		}
		if strings.Contains(strings.ToLower(loc.Err), "permission") || strings.Contains(strings.ToLower(loc.Err), "not permitted") {
			permBlocked = true
		}
		b.WriteString(labelStyle.Render("  "+clamp(shorten(loc.Path), width-8)) + "  " + warnStyle.Render(clamp(loc.Err, 30)) + "\n")
	}
	if permBlocked {
		b.WriteString("\n" + dimStyle.Render("macOS protects the trash: grant your terminal “Full Disk Access”") + "\n")
	}

	restorable := m.restorable
	b.WriteString("\n" + titleStyle.Render(fmt.Sprintf("Restorable by vynsh · %s", plural(int64(len(restorable)), "item", "items"))) + "\n")
	if len(restorable) == 0 {
		b.WriteString(dimStyle.Render("  nothing recorded — items moved here by vynsh can be put back with u") + "\n")
	} else {
		limit := m.height - 18
		if limit < 3 {
			limit = 3
		}
		for i, rec := range restorable {
			if i >= limit {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  … and %d more", len(restorable)-limit)) + "\n")
				break
			}
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				labelStyle.Render(padLeft(units.HumanBytes(rec.Bytes), 9)),
				labelStyle.Render(rec.Time.Format("01-02 15:04")),
				clamp(shorten(rec.Original), width-24)))
		}
		b.WriteString("\n" + labelStyle.Render("u restores these items · enter empties the bin") + "\n")
	}
	return b.String()
}
