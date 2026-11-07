// Package tui renders the interactive dashboard. The look follows the muted,
// boxed style of modern terminal agents: a header, numbered views, a mascot
// panel and a footer that always shows the available keys.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/snapshot"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// Options configures the dashboard.
type Options struct {
	Refresh time.Duration
	NoColor bool
	// Version is shown in the header, e.g. "0.3 beta".
	Version string
}

type tabIndex int

const (
	tabOverview tabIndex = iota
	tabCleanup
	tabDisk
	tabDupes
	tabTrash
	tabCount
)

var tabNames = []string{"Overview", "Cleanup", "Disk", "Duplicates", "Trash"}

type phase int

const (
	phaseIdle phase = iota
	phaseMeasuring
	phaseReady
	phaseApplying
)

type logLine struct {
	at   time.Time
	text string
	kind string // "ok", "warn", "err", "info"
}

// ruleRow is one cleanup rule with its measured size.
type ruleRow struct {
	rule  clean.Rule
	size  int64
	paths int
}

type promptState struct {
	question string
	action   func() tea.Cmd
}

type model struct {
	mon     *sysinfo.Monitor
	refresh time.Duration

	snap     *sysinfo.Snapshot
	scanRes  *scan.Result
	scanning bool

	rules    []ruleRow
	selected map[string]bool
	cursor   int
	phase    phase
	progress struct {
		done, total int
		path        string
	}
	applyRep *clean.Report

	trashRep   *trash.Report
	restorable []trash.Record

	dupes        dupeState
	dupeProgress string

	tab     tabIndex
	width   int
	height  int
	frame   int
	busy    string
	version string

	prompt      *promptState
	beetleX     int
	beetleShift int // columns the beetle has walked away from its corner (<= 0)
	beetleDir   int // -1 walking left, +1 walking right
	beetleWalk  int // ticks left in the current stroll
	happyUntil  int
	logs        []logLine
	quitting    bool
	err         error
	events      chan tea.Msg
}

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
	rows []ruleRow
	err  error
}

type junkProgressMsg struct {
	done, total int
	path        string
}

type trashMsg struct {
	rep     *trash.Report
	records []trash.Record
	err     error
}

type applyMsg struct {
	rep *clean.Report
	err error
}

type restoreMsg struct {
	rep *trash.RestoreReport
	err error
}

type emptyTrashMsg struct {
	rep *trash.Report
	err error
}

type snapshotMsg struct {
	name string
	err  error
}

// Run starts the dashboard and blocks until the user quits.
func Run(opts Options) error {
	if opts.Refresh <= 0 {
		opts.Refresh = 2 * time.Second
	}
	m := newModel(opts)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(opts Options) *model {
	m := &model{
		mon: sysinfo.NewMonitor(sysinfo.Options{
			Interval: 250 * time.Millisecond, TopN: 6, SortBy: "cpu", IncludeProcs: true,
		}),
		refresh:  opts.Refresh,
		version:  opts.Version,
		width:    100,
		height:   32,
		selected: map[string]bool{},
		events:   make(chan tea.Msg, 128),
	}
	m.loadRules()
	return m
}

// loadRules builds the rule list with a safe default selection: low risk only.
func (m *model) loadRules() {
	rules, err := clean.Select(nil, nil, false, false)
	if err != nil {
		m.err = err
		return
	}
	m.rules = make([]ruleRow, 0, len(rules))
	for _, r := range rules {
		m.rules = append(m.rules, ruleRow{rule: r})
		if r.Risk == clean.RiskLow {
			m.selected[r.ID] = true
		}
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.fetchSnap(), m.fetchTrash(), m.tick(), m.measureCmd())
}

func (m *model) tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) fetchSnap() tea.Cmd {
	return func() tea.Msg {
		s, err := m.mon.Collect()
		return snapMsg{snap: s, err: err}
	}
}

func (m *model) fetchTrash() tea.Cmd {
	return func() tea.Msg {
		if !trash.Supported() {
			return trashMsg{err: fmt.Errorf("trash is not supported on this platform")}
		}
		rep, err := trash.Inspect()
		recs, rerr := trash.Restorable()
		if err == nil {
			err = rerr
		}
		return trashMsg{rep: rep, records: recs, err: err}
	}
}

// waitEvent reads one message from the progress channel.
func waitEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// measureCmd walks every visible rule in the background, so the size of a rule
// is known before it is selected for removal.
func (m *model) measureCmd() tea.Cmd {
	m.phase = phaseMeasuring
	m.progress.done, m.progress.total = 0, 0
	rules := make([]clean.Rule, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, r.rule)
	}
	ch := m.events
	return func() tea.Msg {
		targets := clean.Discover(rules, clean.DiscoverOptions{
			Progress: func(done, total int, path string) {
				select {
				case ch <- junkProgressMsg{done: done, total: total, path: path}:
				default:
				}
			},
		})
		byRule := map[string]ruleRow{}
		for _, t := range targets {
			if !t.Exists || t.Size == 0 {
				continue
			}
			row := byRule[t.Rule.ID]
			row.rule = t.Rule
			row.size += t.Size
			row.paths++
			byRule[t.Rule.ID] = row
		}
		rows := make([]ruleRow, 0, len(byRule))
		for _, r := range byRule {
			rows = append(rows, r)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].size > rows[j].size })
		return junkMsg{rows: rows}
	}
}

// mergeMeasured keeps the full rule catalogue (so unselected rules stay
// visible) and fills in the measured sizes.
func (m *model) mergeMeasured(rows []ruleRow) {
	byID := map[string]ruleRow{}
	for _, r := range rows {
		byID[r.rule.ID] = r
	}
	for i := range m.rules {
		if r, ok := byID[m.rules[i].rule.ID]; ok {
			m.rules[i].size = r.size
			m.rules[i].paths = r.paths
			continue
		}
		m.rules[i].size = 0
		m.rules[i].paths = 0
	}
	sort.SliceStable(m.rules, func(i, j int) bool {
		if m.rules[i].size == m.rules[j].size {
			return m.rules[i].rule.ID < m.rules[j].rule.ID
		}
		return m.rules[i].size > m.rules[j].size
	})
}

func (m *model) selectedRules() []clean.Rule {
	var out []clean.Rule
	for _, r := range m.rules {
		if m.selected[r.rule.ID] {
			out = append(out, r.rule)
		}
	}
	return out
}

func (m *model) selectedSize() int64 {
	var sum int64
	for _, r := range m.rules {
		if m.selected[r.rule.ID] {
			sum += r.size
		}
	}
	return sum
}

func (m *model) selectedCount() int {
	n := 0
	for _, r := range m.rules {
		if m.selected[r.rule.ID] {
			n++
		}
	}
	return n
}

// applyCmd removes the selected rules' contents in the background.
func (m *model) applyCmd() tea.Cmd {
	ids := map[string]bool{}
	for _, r := range m.rules {
		if m.selected[r.rule.ID] && r.size > 0 {
			ids[r.rule.ID] = true
		}
	}
	rules := m.selectedRules()
	m.phase = phaseApplying
	m.err = nil
	return func() tea.Msg {
		targets := clean.Discover(rules, clean.DiscoverOptions{})
		present := make([]clean.Target, 0, len(targets))
		for _, t := range targets {
			if ids[t.Rule.ID] && t.Exists && t.Size > 0 {
				present = append(present, t)
			}
		}
		rep := clean.Apply(present, false, func(it clean.Item) {
			entry := audit.Entry{Action: "clean", Path: it.Path, Bytes: it.Bytes, Result: "ok"}
			if it.Err != "" {
				entry.Result = "error"
				entry.Error = it.Err
			}
			_ = audit.Log(entry)
		})
		return applyMsg{rep: rep}
	}
}

func (m *model) scanCmd() tea.Cmd {
	root := fsutil.HomeDir()
	m.scanning = true
	m.busy = "scanning " + shorten(root)
	return func() tea.Msg {
		res, err := scan.Run(scan.Options{Root: root, Depth: 2, TopFiles: 10})
		return scanMsg{res: res, err: err}
	}
}

func (m *model) snapshotCmd(res *scan.Result) tea.Cmd {
	m.busy = "saving snapshot"
	return func() tea.Msg {
		snap, err := snapshot.Save("", res, 2)
		if err != nil {
			return snapshotMsg{err: err}
		}
		return snapshotMsg{name: snap.Name}
	}
}

func (m *model) restoreCmd(records []trash.Record) tea.Cmd {
	m.busy = "restoring"
	return func() tea.Msg {
		rep, err := trash.Restore(records, false)
		for _, r := range rep.Results {
			result := "ok"
			if r.Err != "" {
				result = "error"
			}
			_ = audit.Log(audit.Entry{Action: "trash-restore", Path: r.Record.Original,
				Bytes: r.Record.Bytes, Result: result, Error: r.Err})
		}
		return restoreMsg{rep: rep, err: err}
	}
}

func (m *model) emptyTrashCmd() tea.Cmd {
	m.busy = "emptying the trash"
	return func() tea.Msg {
		rep, err := trash.Empty(false)
		if rep != nil {
			for _, loc := range rep.Locations {
				_ = audit.Log(audit.Entry{Action: "trash-empty", Path: loc.Path, Bytes: loc.Bytes, Result: "ok"})
			}
		}
		return emptyTrashMsg{rep: rep, err: err}
	}
}

func (m *model) addLog(kind, text string) {
	m.logs = append(m.logs, logLine{at: time.Now(), text: text, kind: kind})
	if len(m.logs) > 6 {
		m.logs = m.logs[len(m.logs)-6:]
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.frame++
		m.stepBeetle()
		return m, m.tick()

	case snapMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.snap = msg.snap
			m.err = nil
		}
		return m, nil

	case junkProgressMsg:
		m.progress.done, m.progress.total, m.progress.path = msg.done, msg.total, msg.path
		return m, waitEvent(m.events)

	case junkMsg:
		m.phase = phaseReady
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.mergeMeasured(msg.rows)
		if m.cursor >= len(m.rules) {
			m.cursor = max0(len(m.rules) - 1)
		}
		return m, nil

	case applyMsg:
		m.phase = phaseReady
		m.applyRep = msg.rep
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.rep != nil {
			m.celebrate()
			m.addLog("ok", fmt.Sprintf("freed %s in %s", units.HumanBytes(msg.rep.Bytes),
				plural(int64(len(msg.rep.Removed)), "target", "targets")))
			if len(msg.rep.Failed) > 0 {
				m.addLog("warn", fmt.Sprintf("%d targets could not be removed", len(msg.rep.Failed)))
			}
		}
		return m, tea.Batch(m.fetchSnap(), m.measureCmd())

	case scanMsg:
		m.scanning = false
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.scanRes = msg.res
		m.addLog("ok", fmt.Sprintf("scanned %s — %s", shorten(msg.res.Root), units.HumanBytes(msg.res.Total)))
		return m, nil

	case snapshotMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.addLog("ok", "snapshot saved: "+msg.name)
		return m, nil

	case trashMsg:
		m.trashRep = msg.rep
		m.restorable = msg.records
		if msg.err != nil && m.err == nil {
			m.err = msg.err
		}
		return m, nil

	case restoreMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.rep != nil {
			m.celebrate()
			m.addLog("ok", fmt.Sprintf("restored %s from the trash", units.HumanBytes(msg.rep.Bytes)))
		}
		return m, m.fetchTrash()

	case emptyTrashMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.rep != nil {
			m.celebrate()
			m.addLog("ok", fmt.Sprintf("emptied the trash — %s freed", units.HumanBytes(msg.rep.Bytes)))
		}
		return m, m.fetchTrash()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	if cmd, ok := m.handleDupeMsg(msg); ok {
		return m, cmd
	}
	return m, nil
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.prompt != nil {
		switch key {
		case "y", "Y":
			action := m.prompt.action
			m.prompt = nil
			if action != nil {
				return m, action()
			}
			return m, nil
		case "n", "N", "esc", "q":
			m.prompt = nil
			m.addLog("info", "cancelled")
			return m, nil
		}
		return m, nil
	}

	if m.tab == tabDupes {
		if cmd, ok := m.handleDupeKey(key); ok {
			return m, cmd
		}
	}

	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "1":
		m.tab = tabOverview
	case "2":
		m.tab = tabCleanup
	case "3":
		m.tab = tabDisk
	case "4":
		m.tab = tabDupes
	case "5":
		m.tab = tabTrash
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % tabCount
	case "shift+tab", "left", "h":
		m.tab = (m.tab + tabCount - 1) % tabCount
	case "r":
		return m, tea.Batch(m.fetchSnap(), m.fetchTrash(), m.measureCmd())
	case "s":
		if !m.scanning {
			m.tab = tabDisk
			return m, m.scanCmd()
		}
	case "v":
		if m.scanRes != nil && m.busy == "" {
			return m, m.snapshotCmd(m.scanRes)
		}
		if !m.scanning {
			return m, m.scanCmd()
		}
	case "up", "k":
		if m.tab == tabCleanup && m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.tab == tabCleanup && m.cursor < len(m.rules)-1 {
			m.cursor++
		}
	case " ":
		if m.tab == tabCleanup && m.cursor < len(m.rules) {
			id := m.rules[m.cursor].rule.ID
			m.selected[id] = !m.selected[id]
		}
	case "a":
		if m.tab == tabCleanup {
			for _, r := range m.rules {
				if r.rule.Risk != clean.RiskHigh && !r.rule.Admin {
					m.selected[r.rule.ID] = true
				}
			}
		}
	case "n":
		if m.tab == tabCleanup {
			for _, r := range m.rules {
				m.selected[r.rule.ID] = false
			}
		}
	case "enter":
		switch m.tab {
		case tabCleanup:
			size := m.selectedSize()
			if size == 0 {
				m.addLog("info", "nothing selected — press space to pick rules")
				return m, nil
			}
			count := m.selectedCount()
			m.prompt = &promptState{
				question: fmt.Sprintf("remove %s of caches (%s)?", units.HumanBytes(size),
					plural(int64(count), "rule", "rules")),
				action: func() tea.Cmd { return m.applyCmd() },
			}
		case tabTrash:
			if m.trashRep != nil && m.trashRep.Items > 0 {
				m.prompt = &promptState{
					question: "empty the trash permanently?",
					action:   func() tea.Cmd { return m.emptyTrashCmd() },
				}
			} else {
				m.addLog("info", "trash is empty")
			}
		}
	case "u":
		if m.tab == tabTrash {
			if len(m.restorable) == 0 {
				m.addLog("info", "nothing to restore")
				return m, nil
			}
			records := m.restorable
			m.prompt = &promptState{
				question: fmt.Sprintf("restore %s?", plural(int64(len(records)), "item", "items")),
				action:   func() tea.Cmd { return m.restoreCmd(records) },
			}
		}
	}
	return m, nil
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	width := m.width
	if width < 40 {
		width = 100
	}
	out := lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(width),
		m.tabsView(),
		m.bodyView(width),
		m.footerView(width),
	)
	// Nothing may be wider than the terminal, whatever the active view did.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = clampLine(l, width)
	}
	return strings.Join(lines, "\n")
}

func (m *model) headerView(width int) string {
	subtitle := "  disk cleanup & system monitor"
	if m.version != "" {
		subtitle = " " + m.version + "  disk cleanup & system monitor"
	}
	left := titleStyle.Render("✻ vynsh") + labelStyle.Render(subtitle)
	right := ""
	if m.snap != nil {
		right = labelStyle.Render(m.snap.Hostname + " · " + m.snap.Time.Format("15:04:05"))
	}
	space := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if space < 1 {
		space = 1
	}
	return left + repeat(" ", space) + right
}

func (m *model) tabsView() string {
	parts := make([]string, 0, tabCount)
	for i := 0; i < int(tabCount); i++ {
		label := fmt.Sprintf("%d %s", i+1, tabNames[i])
		if tabIndex(i) == m.tab {
			parts = append(parts, activeTab.Render(label))
		} else {
			parts = append(parts, inactiveTab.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

// bodyView renders the active view at full width.
func (m *model) bodyView(width int) string {
	var content string
	switch m.tab {
	case tabOverview:
		content = m.overviewView()
	case tabCleanup:
		content = m.cleanupView()
	case tabDisk:
		content = m.diskView()
	case tabDupes:
		content = m.dupesView()
	case tabTrash:
		content = m.trashView()
	}
	return content
}

func (m *model) isBusy() bool {
	return m.scanning || m.busy != "" || m.phase == phaseMeasuring || m.phase == phaseApplying
}

// speech is the mascot's current line of dialogue.
func (m *model) speech() string {
	switch {
	case m.phase == phaseMeasuring:
		if m.progress.total > 0 {
			return fmt.Sprintf("measuring caches %d/%d…", m.progress.done, m.progress.total)
		}
		return "measuring caches…"
	case m.phase == phaseApplying:
		return "cleaning up…"
	case m.scanning:
		return "scanning your disk…"
	case m.busy != "":
		return m.busy + "…"
	case m.tab == tabCleanup:
		if s := m.selectedSize(); s > 0 {
			return fmt.Sprintf("%s selected — enter?", units.HumanBytes(s))
		}
		return "pick something to clean"
	case m.tab == tabDisk:
		return "press s to scan"
	case m.tab == tabDupes:
		if m.dupes.scanned && len(m.dupes.groups) > 0 {
			return fmt.Sprintf("%s to free up", units.HumanBytes(m.dupes.wasted))
		}
		return "press s to find duplicates"
	case m.tab == tabTrash:
		if m.trashRep != nil && m.trashRep.Items > 0 {
			return fmt.Sprintf("%s in the bin", units.HumanBytes(m.trashRep.Bytes))
		}
		return "bin is empty"
	}
	if m.snap != nil {
		return fmt.Sprintf("cpu %s · mem %s", units.Percent(m.snap.CPU.Percent, 0), units.Percent(m.snap.Mem.Percent, 0))
	}
	return "everything is calm"
}

func (m *model) footerView(width int) string {
	keys := "1-5 views · tab/←→ switch · r refresh · s scan · q quit"
	switch m.tab {
	case tabCleanup:
		keys = "↑↓ move · space toggle · a all safe · n none · enter apply · r re-measure · q quit"
	case tabDisk:
		keys = "s scan home · v save snapshot · → next view · q quit"
	case tabDupes:
		keys = "↑↓ move · space mark copy · a mark all extra · n clear · enter trash marked · s search · q quit"
	case tabTrash:
		keys = "u restore items · enter empty bin · r refresh · q quit"
	}
	status := m.statusLine()
	body := labelStyle.Render(keys)
	if status != "" {
		body = status + "\n" + body
	}
	return m.withBeetle(boxStyle.Width(width-4).Render(body), width)
}

func (m *model) statusLine() string {
	if m.prompt != nil {
		return warnStyle.Render("? " + m.prompt.question + "  [y/N]")
	}
	if m.err != nil {
		return badStyle.Render("✗ " + clamp(m.err.Error(), m.width-8))
	}
	switch {
	case m.phase == phaseMeasuring:
		return accentStyle.Render(spinner(m.frame)+" measuring caches") + " " + labelStyle.Render(m.progressLabel())
	case m.phase == phaseApplying:
		return accentStyle.Render(spinner(m.frame) + " cleaning…")
	case m.scanning:
		return accentStyle.Render(spinner(m.frame) + " scanning " + shorten(fsutil.HomeDir()))
	case m.busy != "":
		return accentStyle.Render(spinner(m.frame)+" "+m.busy) + "…"
	}
	if len(m.logs) > 0 {
		last := m.logs[len(m.logs)-1]
		icon := goodStyle.Render("✓")
		switch last.kind {
		case "warn":
			icon = warnStyle.Render("!")
		case "err":
			icon = badStyle.Render("✗")
		case "info":
			icon = labelStyle.Render("·")
		}
		return fmt.Sprintf("%s %s %s", icon, textStyle.Render(clamp(last.text, m.width-20)),
			labelStyle.Render(last.at.Format("15:04:05")))
	}
	return ""
}

func (m *model) progressLabel() string {
	if m.progress.total > 0 {
		return fmt.Sprintf("%d/%d", m.progress.done, m.progress.total)
	}
	return ""
}

// plural renders "1 rule" / "3 rules".
func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return units.HumanCount(n) + " " + many
}

// shorten makes an absolute path compact.
func shorten(p string) string {
	if h := fsutil.HomeDir(); h != "" && strings.HasPrefix(p, h) {
		p = "~" + strings.TrimPrefix(p, h)
	}
	return p
}

// stepBeetle lets the mascot stroll a few columns along the input box now and
// then. The range is deliberately short: the beetle stays in its corner instead
// of wandering across the screen.
func (m *model) stepBeetle() {
	const leftmost = -4
	if m.beetleDir == 0 {
		m.beetleDir = -1
	}
	if m.beetleWalk > 0 {
		if m.frame%3 != 0 {
			return
		}
		m.beetleWalk--
		m.beetleShift += m.beetleDir
		switch {
		case m.beetleShift <= leftmost:
			m.beetleShift = leftmost
			m.beetleDir = 1
		case m.beetleShift >= 0:
			m.beetleShift = 0
			m.beetleDir = -1
		}
		return
	}
	if !m.isBusy() && m.frame%80 == 0 {
		m.beetleWalk = 12
	}
}

// celebrate makes the beetle hop twice after a successful cleanup.
func (m *model) celebrate() {
	m.happyUntil = m.frame + 14
}

// clampLine cuts a rendered line down to w visible cells, keeping ANSI codes.
func clampLine(line string, w int) string {
	if lipgloss.Width(line) <= w {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(line)
}
