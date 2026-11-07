// Package tui implements the interactive dashboard: a live resource overview,
// a disk usage view and a junk view, refreshed while it runs.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// Options configures the dashboard.
type Options struct {
	Refresh time.Duration
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	goodStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	badStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type tickMsg time.Time

type snapMsg struct {
	snap *sysinfo.Snapshot
	err  error
}

type scanMsg struct {
	res *scan.Result
	err error
}

type junkMsg struct {
	targets []clean.Target
	err     error
}

type trashMsg struct {
	rep *trash.Report
	err error
}

type model struct {
	mon      *sysinfo.Monitor
	refresh  time.Duration
	snap     *sysinfo.Snapshot
	scanRes  *scan.Result
	junk     []clean.Target
	junkSize int64
	trashRep *trash.Report
	tab      int
	width    int
	height   int
	err      error
	busy     string
	quitting bool
}

// Run starts the dashboard and blocks until the user quits.
func Run(opts Options) error {
	if opts.Refresh <= 0 {
		opts.Refresh = 2 * time.Second
	}
	m := &model{
		mon:     sysinfo.NewMonitor(sysinfo.Options{Interval: 250 * time.Millisecond, TopN: 8, SortBy: "cpu", IncludeProcs: true}),
		refresh: opts.Refresh,
		width:   100,
		height:  30,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.fetchSnap(), m.fetchJunk(), m.fetchTrash(), m.tick())
}

func (m *model) tick() tea.Cmd {
	return tea.Tick(m.refresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) fetchSnap() tea.Cmd {
	return func() tea.Msg {
		s, err := m.mon.Collect()
		return snapMsg{snap: s, err: err}
	}
}

func (m *model) fetchJunk() tea.Cmd {
	return func() tea.Msg {
		rules, err := clean.Select(nil, nil, false)
		if err != nil {
			return junkMsg{err: err}
		}
		targets := clean.Discover(rules, clean.DiscoverOptions{})
		out := make([]clean.Target, 0, len(targets))
		for _, t := range targets {
			if t.Exists && t.Size > 0 {
				out = append(out, t)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
		return junkMsg{targets: out}
	}
}

func (m *model) fetchTrash() tea.Cmd {
	return func() tea.Msg {
		if !trash.Supported() {
			return trashMsg{}
		}
		rep, err := trash.Inspect()
		return trashMsg{rep: rep, err: err}
	}
}

func (m *model) startScan() tea.Cmd {
	root := fsutil.HomeDir()
	m.busy = "scanning " + root
	return func() tea.Msg {
		res, err := scan.Run(scan.Options{Root: root, Depth: 2, TopFiles: 10})
		return scanMsg{res: res, err: err}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetchSnap(), m.tick())
	case snapMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.snap = msg.snap
			m.err = nil
		}
		return m, nil
	case scanMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.scanRes = msg.res
		}
		return m, nil
	case junkMsg:
		m.busy = ""
		m.junk = msg.targets
		m.junkSize = clean.Total(msg.targets)
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil
	case trashMsg:
		if msg.err == nil {
			m.trashRep = msg.rep
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			return m, tea.Batch(m.fetchSnap(), m.fetchJunk(), m.fetchTrash())
		case "s":
			if m.busy == "" {
				return m, m.startScan()
			}
		case "1":
			m.tab = 0
		case "2":
			m.tab = 1
		case "3":
			m.tab = 2
		case "tab":
			m.tab = (m.tab + 1) % 3
		}
	}
	return m, nil
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.tabs())
	b.WriteString("\n")
	switch m.tab {
	case 1:
		b.WriteString(m.diskView())
	case 2:
		b.WriteString(m.junkView())
	default:
		b.WriteString(m.overview())
	}
	if m.err != nil {
		b.WriteString("\n" + badStyle.Render("error: "+m.err.Error()))
	}
	if m.busy != "" {
		b.WriteString("\n" + warnStyle.Render("⏳ "+m.busy))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("q quit · r refresh · s scan home · 1/2/3 or tab switch views"))
	return b.String()
}

func (m *model) tabs() string {
	names := []string{" 1 Overview ", " 2 Disk usage ", " 3 Junk "}
	parts := make([]string, len(names))
	for i, n := range names {
		if i == m.tab {
			parts[i] = activeTab.Render("▸" + n)
		} else {
			parts[i] = inactiveTab.Render(n)
		}
	}
	right := ""
	if m.snap != nil {
		right = dimStyle.Render(m.snap.Time.Format("15:04:05") + " · " + m.snap.Hostname)
	}
	line := strings.Join(parts, " ")
	if pad := m.innerWidth() - lipgloss.Width(line) - lipgloss.Width(right); pad > 0 {
		line = line + strings.Repeat(" ", pad) + right
	}
	return titleStyle.Render("vynsh") + " " + line
}

func (m *model) innerWidth() int {
	w := m.width
	if w < 40 {
		w = 100
	}
	return w - 2
}

func bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	body := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	st := goodStyle
	switch {
	case pct >= 90:
		st = badStyle
	case pct >= 75:
		st = warnStyle
	}
	return st.Render(body)
}

func row(label, value string) string {
	return fmt.Sprintf("%s %s", labelStyle.Render(label), value)
}

func (m *model) overview() string {
	if m.snap == nil {
		return dimStyle.Render("collecting the first sample…")
	}
	s := m.snap
	// lipgloss Width() includes the padding but not the border, so a box laid
	// out at width w occupies w+2 cells and leaves w-2 cells for content.
	half := m.innerWidth() / 2
	cw := half - 6
	if cw < 20 {
		cw = 20
	}
	fullW := m.innerWidth() - 5
	fullContent := fullW - 2

	cpuHead := clamp(fmt.Sprintf("%s · %d cores · %s", s.CPU.Model, s.CPU.Cores,
		valueStyle.Render(units.Percent(s.CPU.Percent, 1))), cw)
	cpuLoad := "load —"
	if s.CPU.Load.L1 > 0 {
		cpuLoad = fmt.Sprintf("load %.2f %.2f %.2f", s.CPU.Load.L1, s.CPU.Load.L5, s.CPU.Load.L15)
	}
	cpu := boxStyle.Width(cw + 2).Render(strings.Join([]string{
		titleStyle.Render("CPU"),
		cpuHead,
		clamp(dimStyle.Render(cpuLoad), cw),
		bar(s.CPU.Percent, cw),
	}, "\n"))

	memHead := clamp(fmt.Sprintf("%s / %s · %s", valueStyle.Render(units.HumanBytes(int64(s.Mem.Used))),
		units.HumanBytes(int64(s.Mem.Total)), units.Percent(s.Mem.Percent, 1)), cw)
	memSwap := "swap not used"
	if s.Mem.SwapTotal > 0 {
		memSwap = fmt.Sprintf("swap %s / %s", units.HumanBytes(int64(s.Mem.SwapUsed)), units.HumanBytes(int64(s.Mem.SwapTotal)))
	}
	mem := boxStyle.Width(cw + 2).Render(strings.Join([]string{
		titleStyle.Render("Memory"),
		memHead,
		clamp(labelStyle.Render(memSwap), cw),
		bar(s.Mem.Percent, cw),
	}, "\n"))

	var b strings.Builder
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cpu, mem))

	var disks []string
	for _, d := range s.Disks {
		disks = append(disks, fmt.Sprintf("%s %s %s %s", labelStyle.Render(padRight(d.Mountpoint, 16)),
			bar(d.Percent, 20), valueStyle.Render(units.HumanBytes(int64(d.Free))), dimStyle.Render("free of "+units.HumanBytes(int64(d.Total)))))
	}
	for i, d := range disks {
		disks[i] = clamp(d, fullContent)
	}
	if len(disks) > 0 {
		b.WriteString("\n" + boxStyle.Width(fullW).Render(titleStyle.Render("Disks")+"\n"+strings.Join(disks, "\n")))
	}

	var net []string
	for _, n := range s.Net {
		if n.SendRate == 0 && n.RecvRate == 0 {
			continue
		}
		net = append(net, fmt.Sprintf("%s ↓ %s ↑ %s", padRight(n.Name, 12), units.HumanRate(n.RecvRate), units.HumanRate(n.SendRate)))
	}
	for i, n := range net {
		net[i] = clamp(n, fullContent)
	}
	if len(net) > 0 {
		b.WriteString("\n" + boxStyle.Width(fullW).Render(titleStyle.Render("Network")+"\n"+strings.Join(net, "\n")))
	}

	if len(s.Procs) > 0 {
		rows := make([]string, 0, len(s.Procs))
		for _, p := range s.Procs {
			rows = append(rows, fmt.Sprintf("%s %s %s %s",
				labelStyle.Render(padLeft(units.HumanCount(int64(p.PID)), 7)),
				padRight(units.Percent(p.CPU, 1), 7),
				padRight(units.HumanBytes(int64(p.RSS)), 10),
				p.Name))
		}
		for i, r := range rows {
			rows[i] = clamp(r, fullContent)
		}
		b.WriteString("\n" + boxStyle.Width(fullW).Render(
			titleStyle.Render(fmt.Sprintf("Top processes (%d running)", s.ProcCount))+"\n"+strings.Join(rows, "\n")))
	}
	return b.String()
}

func (m *model) diskView() string {
	var b strings.Builder
	if m.scanRes == nil {
		b.WriteString(dimStyle.Render("No scan yet — press s to analyse your home directory."))
		if m.busy != "" {
			b.WriteString("\n" + warnStyle.Render("scanning, this can take a moment…"))
		}
		return b.String()
	}
	r := m.scanRes
	b.WriteString(fmt.Sprintf("%s  %s · %s files\n\n", valueStyle.Render(r.Root), valueStyle.Render(units.HumanBytes(r.Total)), units.HumanCount(r.Files)))
	b.WriteString(dimStyle.Render(padRight("SIZE", 10)+" "+padRight("SHARE", 7)+" "+"PATH") + "\n")
	nodes := r.TopDirs
	if len(nodes) > 18 {
		nodes = nodes[:18]
	}
	for _, n := range nodes {
		pct := 0.0
		if r.Total > 0 {
			pct = float64(n.Size) / float64(r.Total) * 100
		}
		rel := n.Path
		if len(rel) > 60 {
			rel = "…" + rel[len(rel)-59:]
		}
		b.WriteString(fmt.Sprintf("%s %s %s %s\n",
			valueStyle.Render(padLeft(units.HumanBytes(n.Size), 10)),
			dimStyle.Render(padLeft(units.Percent(pct, 1), 6)),
			bar(pct, 12),
			rel))
	}
	if len(r.TopFiles) > 0 {
		b.WriteString("\n" + titleStyle.Render("Largest files") + "\n")
		for _, f := range r.TopFiles {
			rel := f.Path
			if len(rel) > 70 {
				rel = "…" + rel[len(rel)-69:]
			}
			b.WriteString(fmt.Sprintf("%s %s\n", valueStyle.Render(padLeft(units.HumanBytes(f.Size), 10)), rel))
		}
	}
	return b.String()
}

func (m *model) junkView() string {
	var b strings.Builder
	trashLine := dimStyle.Render("trash: unknown")
	if m.trashRep != nil {
		trashLine = fmt.Sprintf("%s %s in %s items", labelStyle.Render("trash:"),
			valueStyle.Render(units.HumanBytes(m.trashRep.Bytes)), units.HumanCount(m.trashRep.Items))
	}
	b.WriteString(fmt.Sprintf("%s  %s %s\n\n", labelStyle.Render("reclaimable:"),
		valueStyle.Render(units.HumanBytes(m.junkSize)), trashLine))
	if len(m.junk) == 0 {
		b.WriteString(dimStyle.Render("Nothing found yet (measuring…), or your cache directories are empty."))
		return b.String()
	}
	b.WriteString(dimStyle.Render(padRight("SIZE", 10)+" "+padRight("RISK", 7)+" "+"WHAT") + "\n")
	limit := 18
	for i, t := range m.junk {
		if i >= limit {
			b.WriteString(dimStyle.Render(fmt.Sprintf("… and %d more targets", len(m.junk)-limit)) + "\n")
			break
		}
		risk := t.Rule.Risk.String()
		switch t.Rule.Risk {
		case clean.RiskHigh:
			risk = badStyle.Render(risk)
		case clean.RiskMedium:
			risk = warnStyle.Render(risk)
		default:
			risk = goodStyle.Render(risk)
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n",
			valueStyle.Render(padLeft(units.HumanBytes(t.Size), 10)), padRight(risk, 15), shorten(t.Path)))
	}
	b.WriteString("\n" + dimStyle.Render("Run `vynsh clean --yes` in a terminal to remove these."))
	return b.String()
}

func padRight(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func padLeft(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d) + s
}

// shorten makes an absolute path compact enough for a dashboard row.
func shorten(p string) string {
	if h := fsutil.HomeDir(); h != "" && strings.HasPrefix(p, h) {
		p = "~" + strings.TrimPrefix(p, h)
	}
	if len(p) > 70 {
		p = "…" + p[len(p)-69:]
	}
	return p
}

// clamp shortens s so that it fits into w terminal cells.
func clamp(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
