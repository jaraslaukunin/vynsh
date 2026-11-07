package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/dupes"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// dupeRow is one line of the duplicates list. Headers describe a group; file
// rows can be marked for removal.
type dupeRow struct {
	header bool
	group  int // index into dupeState.groups
	path   string
	size   int64
	count  int // copies in the group (header rows)
}

// dupeState holds the duplicate search result and the user's marks.
type dupeState struct {
	groups  []dupes.Group
	rows    []dupeRow
	marked  map[string]bool // path -> marked for removal
	cursor  int             // index into rows; always on a file row
	scanned bool
	running bool
	files   int64
	found   int
	wasted  int64
	elapsed float64
	lastRun string
}

type dupeProgressMsg struct {
	phase       string
	done, total int64
}

type dupeScanMsg struct {
	res *dupes.Result
	err error
}

type dupeRemoveMsg struct {
	moved int
	bytes int64
	err   error
}

// startDupeScan searches the home directory for identical files in the
// background, reporting progress through the event channel.
func (m *model) startDupeScan() tea.Cmd {
	root := fsutil.HomeDir()
	m.dupes.running = true
	m.dupes.lastRun = root
	m.busy = "searching duplicates"
	ch := m.events
	return tea.Batch(waitEvent(ch), func() tea.Msg {
		res, err := dupes.Find([]string{root}, dupes.Options{
			MinSize: 1 << 20,
			Progress: func(phase string, done, total int64) {
				select {
				case ch <- dupeProgressMsg{phase: phase, done: done, total: total}:
				default:
				}
			},
		})
		return dupeScanMsg{res: res, err: err}
	})
}

// buildDupeRows flattens groups into list rows and pre-marks every copy except
// the first one, so a single Enter removes the obvious redundancy and always
// keeps one copy per group.
func (d *dupeState) build(groups []dupes.Group) {
	d.groups = groups
	d.rows = d.rows[:0]
	d.marked = map[string]bool{}
	for gi, g := range groups {
		d.rows = append(d.rows, dupeRow{header: true, group: gi, size: g.Size, count: len(g.Paths)})
		for i, p := range g.Paths {
			d.rows = append(d.rows, dupeRow{group: gi, path: p, size: g.Size})
			if i > 0 {
				d.marked[p] = true
			}
		}
	}
	d.cursor = d.firstFileRow()
}

func (d *dupeState) firstFileRow() int {
	for i, r := range d.rows {
		if !r.header {
			return i
		}
	}
	return 0
}

// move steps the cursor by delta over file rows only.
func (d *dupeState) move(delta int) {
	if len(d.rows) == 0 {
		return
	}
	i := d.cursor
	for {
		i += delta
		if i < 0 || i >= len(d.rows) {
			return
		}
		if !d.rows[i].header {
			d.cursor = i
			return
		}
	}
}

// markedBytes sums the size of everything marked for removal.
func (d *dupeState) markedBytes() (int64, int) {
	var sum int64
	n := 0
	for _, g := range d.groups {
		for _, p := range g.Paths {
			if d.marked[p] {
				sum += g.Size
				n++
			}
		}
	}
	return sum, n
}

// safeMarks returns the marked paths, but drops any group that would lose all
// of its copies. This is the guard that makes "remove duplicates" safe.
func (d *dupeState) safeMarks() ([]string, []string) {
	var remove, blocked []string
	for _, g := range d.groups {
		var keep, drop []string
		for _, p := range g.Paths {
			if d.marked[p] {
				drop = append(drop, p)
			} else {
				keep = append(keep, p)
			}
		}
		if len(drop) == 0 {
			continue
		}
		if len(keep) == 0 {
			blocked = append(blocked, g.Paths[0])
			continue
		}
		remove = append(remove, drop...)
	}
	sort.Strings(remove)
	return remove, blocked
}

// removeDupesCmd moves the marked copies to the trash (reversible) and records
// each move in the audit log.
func (m *model) removeDupesCmd(paths []string) tea.Cmd {
	m.busy = "moving duplicates to the trash"
	return func() tea.Msg {
		rep, err := trash.Move(paths, false)
		if err != nil {
			return dupeRemoveMsg{err: err}
		}
		out := dupeRemoveMsg{}
		for _, r := range rep.Moved {
			result := "ok"
			if r.Err != "" {
				result = "error"
			} else {
				out.moved++
				out.bytes += r.Bytes
			}
			_ = audit.Log(audit.Entry{Action: "dupes-trash", Path: r.Path, Bytes: r.Bytes, Result: result, Error: r.Err})
		}
		return out
	}
}

// handleDupeKey implements the keys of the duplicates tab. It returns true when
// the key was consumed.
func (m *model) handleDupeKey(key string) (tea.Cmd, bool) {
	d := &m.dupes
	switch key {
	case "up", "k":
		d.move(-1)
		return nil, true
	case "down", "j":
		d.move(1)
		return nil, true
	case " ":
		if len(d.rows) > 0 && !d.rows[d.cursor].header {
			p := d.rows[d.cursor].path
			d.marked[p] = !d.marked[p]
		}
		return nil, true
	case "a":
		for _, g := range d.groups {
			for i, p := range g.Paths {
				d.marked[p] = i > 0 // keep the first copy of each group
			}
		}
		return nil, true
	case "n":
		d.marked = map[string]bool{}
		return nil, true
	case "s":
		if !d.running && m.busy == "" {
			return m.startDupeScan(), true
		}
		return nil, true
	case "enter":
		if len(d.groups) == 0 {
			m.addLog("info", "no duplicates to remove — press s to search")
			return nil, true
		}
		paths, blocked := d.safeMarks()
		if len(blocked) > 0 {
			m.addLog("warn", fmt.Sprintf("%s would lose every copy — keeping them",
				plural(int64(len(blocked)), "group", "groups")))
		}
		if len(paths) == 0 {
			m.addLog("info", "nothing marked — press space on a copy to mark it")
			return nil, true
		}
		var bytes int64
		for _, p := range paths {
			bytes += m.sizeOfPath(p)
		}
		m.prompt = &promptState{
			question: fmt.Sprintf("move %s to the trash (%s)?", plural(int64(len(paths)), "copy", "copies"), units.HumanBytes(bytes)),
			action:   func() tea.Cmd { return m.removeDupesCmd(paths) },
		}
		return nil, true
	}
	return nil, false
}

// sizeOfPath looks up the size recorded for a duplicate path.
func (m *model) sizeOfPath(p string) int64 {
	for _, g := range m.dupes.groups {
		for _, q := range g.Paths {
			if q == p {
				return g.Size
			}
		}
	}
	return 0
}

// dupesView renders the duplicates tab.
func (m *model) dupesView() string {
	width := m.contentWidth()
	d := &m.dupes
	var right string
	if d.scanned {
		bytes, n := d.markedBytes()
		right = labelStyle.Render(fmt.Sprintf("%s recoverable · %s marked (%s)",
			units.HumanBytes(d.wasted), plural(int64(n), "copy", "copies"), units.HumanBytes(bytes)))
	}
	head := section("Duplicates", right, width)

	if d.running {
		return head + "\n" + accentStyle.Render(spinner(m.frame)+" searching "+shorten(fsutil.HomeDir())+"…") + "\n" +
			dimStyle.Render(fmt.Sprintf("  %s · %s", m.dupeProgress, units.HumanCount(d.files)+" files checked"))
	}
	if !d.scanned {
		return head + "\n" + textStyle.Render("Find files with identical content in your home folder.") + "\n" +
			labelStyle.Render("Press ") + accentStyle.Render("s") + labelStyle.Render(" to search (files of 1 MB and more).") + "\n" +
			labelStyle.Render("Nothing is deleted: marked copies go to the trash, and one copy per group is always kept.")
	}
	if len(d.groups) == 0 {
		return head + "\n" + goodStyle.Render("✓ no duplicates found") + "\n" +
			labelStyle.Render(fmt.Sprintf("checked %s files in %.1fs", units.HumanCount(d.files), d.elapsed))
	}

	var b strings.Builder
	b.WriteString(head + "\n")
	limit := m.height - 14
	if limit < 6 {
		limit = 6
	}
	start := 0
	if d.cursor >= limit {
		start = d.cursor - limit + 1
	}
	end := start + limit
	if end > len(d.rows) {
		end = len(d.rows)
	}
	for i := start; i < end; i++ {
		r := d.rows[i]
		if r.header {
			b.WriteString(titleStyle.Render(fmt.Sprintf("%s × %d", units.HumanBytes(r.size), r.count)) +
				labelStyle.Render(fmt.Sprintf("  saves %s", units.HumanBytes(r.size*int64(r.count-1)))) + "\n")
			continue
		}
		mark := checkboxOff.Render("[ ]")
		if d.marked[r.path] {
			mark = checkboxOn.Render("[x]")
		}
		pointer := "    "
		if i == d.cursor {
			pointer = cursorMark.Render("  ▸ ")
		}
		name := clamp(shorten(r.path), width-12)
		if i == d.cursor {
			name = valueStyle.Render(name)
		} else {
			name = textStyle.Render(name)
		}
		b.WriteString(pointer + mark + " " + name + "\n")
	}
	if len(d.rows) > limit {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  rows %d–%d of %d · ↑↓ to scroll", start+1, end, len(d.rows))) + "\n")
	}
	b.WriteString(labelStyle.Render("The first copy of every group is kept; the other copies are marked by default."))
	return b.String()
}

// handleDupeMsg updates the duplicates state from background messages.
func (m *model) handleDupeMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case dupeProgressMsg:
		m.dupeProgress = fmt.Sprintf("%s %s", msg.phase, progressCount(msg.done, msg.total))
		m.dupes.files = msg.done
		return waitEvent(m.events), true
	case dupeScanMsg:
		m.dupes.running = false
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return nil, true
		}
		m.dupes.scanned = true
		m.dupes.files = msg.res.FilesScanned
		m.dupes.found = len(msg.res.Groups)
		m.dupes.wasted = msg.res.Wasted
		m.dupes.elapsed = msg.res.Elapsed
		m.dupes.build(msg.res.Groups)
		m.addLog("ok", fmt.Sprintf("found %s — %s recoverable",
			plural(int64(len(msg.res.Groups)), "duplicate group", "duplicate groups"),
			units.HumanBytes(msg.res.Wasted)))
		return nil, true
	case dupeRemoveMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err
			return nil, true
		}
		m.celebrate()
		m.addLog("ok", fmt.Sprintf("moved %s to the trash — %s",
			plural(int64(msg.moved), "copy", "copies"), units.HumanBytes(msg.bytes)))
		// Drop the removed paths from the list and refresh the trash view.
		m.dropRemoved()
		return m.fetchTrash(), true
	}
	return nil, false
}

// dropRemoved removes trashed copies from the groups, keeping the list honest.
func (m *model) dropRemoved() {
	var groups []dupes.Group
	for _, g := range m.dupes.groups {
		var keep []string
		for _, p := range g.Paths {
			if fsutil.Exists(p) {
				keep = append(keep, p)
			}
		}
		if len(keep) >= 2 {
			g.Paths = keep
			groups = append(groups, g)
		}
	}
	m.dupes.wasted = 0
	for _, g := range groups {
		m.dupes.wasted += g.Wasted()
	}
	m.dupes.build(groups)
}

func progressCount(done, total int64) string {
	if total > 0 {
		return fmt.Sprintf("%s/%s", units.HumanCount(done), units.HumanCount(total))
	}
	return units.HumanCount(done)
}
