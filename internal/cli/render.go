package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/del"
	"github.com/jaraslaukunin/vynsh/internal/dupes"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// ---- size helpers -------------------------------------------------------

func share(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func colorShare(th *style.Theme, s string, pct float64) string {
	switch {
	case pct >= 50:
		return th.Red(s)
	case pct >= 20:
		return th.Yellow(s)
	default:
		return s
	}
}

// ---- disk scan ----------------------------------------------------------

type treeOptions struct {
	Bars     bool
	MinSize  int64
	MaxRows  int
	MaxDepth int
}

// printScan renders a scan result: header, directory tree, largest files and
// file type breakdown.
func printScan(w io.Writer, res *scan.Result, opts treeOptions) {
	th := g.th
	if opts.MaxRows <= 0 {
		opts.MaxRows = 40
	}
	fmt.Fprintf(w, "%s  %s · %s files · %s dirs · %.1fs\n",
		th.Title(res.Root),
		th.Bold(units.HumanBytes(res.Total)),
		units.HumanCount(res.Files),
		units.HumanCount(res.Dirs),
		res.Elapsed,
	)
	if res.Volume != nil && res.Volume.Total > 0 {
		used := float64(res.Volume.Used) / float64(res.Volume.Total) * 100
		fmt.Fprintf(w, "%s %s total · %s free (%s used)%s\n",
			th.Dim("volume:"),
			units.HumanBytes(int64(res.Volume.Total)),
			units.HumanBytes(int64(res.Volume.Free)),
			units.Percent(used, 0),
			func() string {
				if res.Volume.FSType != "" {
					return " · " + res.Volume.FSType
				}
				return ""
			}(),
		)
	}
	fmt.Fprintln(w)

	if res.Volume != nil && res.Total > int64(res.Volume.Used) {
		fmt.Fprintf(w, "%s\n", th.Dim("note: the total exceeds the volume usage — the tree contains sparse or "+
			"copy-on-write cloned files, which count by logical size."))
	}
	if res.Tree != nil && res.Total > 0 {
		rows := 0
		printTree(w, res.Tree, res.Total, "", "", true, 0, opts, &rows)
		fmt.Fprintln(w)
	}

	if len(res.TopFiles) > 0 {
		fmt.Fprintf(w, "%s\n", th.Title("Largest files"))
		rows := make([][]string, 0, len(res.TopFiles))
		for _, f := range res.TopFiles {
			rows = append(rows, []string{
				units.HumanBytes(f.Size),
				f.Path,
				age(f.ModTime),
			})
		}
		t := &style.Table{
			Headers:  []string{"SIZE", "PATH", "MODIFIED"},
			Rows:     rows,
			Align:    []style.Align{style.Right, style.Left, style.Right},
			MaxWidth: []int{10, 90, 16},
		}
		fmt.Fprint(w, t.Render(th))
		fmt.Fprintln(w)
	}

	if len(res.ByExt) > 0 {
		fmt.Fprintf(w, "%s\n", th.Title("By file type"))
		rows := make([][]string, 0, len(res.ByExt))
		for _, e := range res.ByExt {
			rows = append(rows, []string{
				units.HumanBytes(e.Size),
				units.Percent(share(e.Size, res.Total), 1),
				e.Ext,
				units.HumanCount(e.Count),
			})
		}
		t := &style.Table{
			Headers: []string{"SIZE", "SHARE", "TYPE", "FILES"},
			Rows:    rows,
			Align:   []style.Align{style.Right, style.Right, style.Left, style.Right},
		}
		fmt.Fprint(w, t.Render(th))
		fmt.Fprintln(w)
	}

	if len(res.Skipped) > 0 {
		fmt.Fprintf(w, "%s %d paths could not be read (permissions)\n", th.Dim("note:"), len(res.Skipped))
		if g.verbose {
			for _, s := range res.Skipped {
				fmt.Fprintf(w, "  %s: %s\n", s.Path, th.Dim(s.Err))
			}
		}
	}
}

// printTree walks the aggregated tree and prints it with sizes and shares.
func printTree(w io.Writer, n *scan.Node, total int64, prefix string, connector string, isRoot bool, level int, o treeOptions, rows *int) {
	if *rows >= o.MaxRows {
		return
	}
	pct := share(n.Size, total)
	bar := ""
	if o.Bars {
		bar = style.Bar(pct/100, 12, g.th) + "  "
	}
	sizeStr := colorShare(g.th, units.HumanBytes(n.Size), pct)
	name := n.Name
	if !isRoot {
		name = prefix + connector + name
	}
	label := name
	if n.IsDir && !isRoot {
		label = g.th.Bold(n.Name)
		if prefix != "" || connector != "" {
			label = prefix + connector + label
		}
	}
	if isRoot {
		label = g.th.Bold(g.th.Cyan(n.Name))
	}
	fmt.Fprintf(w, "%12s  %6s  %s%s\n", sizeStr, units.Percent(pct, 1), bar, label)
	*rows++

	if !n.IsDir || level >= o.MaxDepth {
		return
	}
	childPrefix := prefix
	if !isRoot {
		if connector == "└─ " {
			childPrefix = prefix + "   "
		} else {
			childPrefix = prefix + "│  "
		}
	}
	children := make([]*scan.Node, 0, len(n.Children))
	var childSum, childFiles int64
	var hiddenBytes int64
	var hiddenCount int
	for _, c := range n.Children {
		childSum += c.Size
		childFiles += c.Files
		if c.Size < o.MinSize {
			hiddenBytes += c.Size
			hiddenCount++
			continue
		}
		children = append(children, c)
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].Size > children[j].Size })
	for i, c := range children {
		conn := "├─ "
		if i == len(children)-1 && hiddenCount == 0 {
			conn = "└─ "
		}
		printTree(w, c, total, childPrefix, conn, false, level+1, o, rows)
	}
	if hiddenCount > 0 && *rows < o.MaxRows {
		fmt.Fprintf(w, "%12s  %6s  %s%s\n",
			g.th.Dim(units.HumanBytes(hiddenBytes)),
			g.th.Dim(units.Percent(share(hiddenBytes, total), 1)),
			strings.Repeat(" ", func() int {
				if o.Bars {
					return 14
				}
				return 0
			}()),
			childPrefix+"└─ "+g.th.Dim(fmt.Sprintf("%d smaller entries (below threshold)", hiddenCount)))
		*rows++
	}
	// Files stored directly in this directory are not part of the child list.
	direct := n.Size - childSum
	if direct > 0 && *rows < o.MaxRows {
		directFiles := n.Files - childFiles
		label := fmt.Sprintf("(%s files stored right here)", units.HumanCount(directFiles))
		if directFiles == 1 {
			label = "(1 file stored right here)"
		}
		pad := ""
		if o.Bars {
			pad = strings.Repeat(" ", 14)
		}
		fmt.Fprintf(w, "%12s  %6s  %s%s\n",
			g.th.Dim(units.HumanBytes(direct)),
			g.th.Dim(units.Percent(share(direct, total), 1)),
			pad,
			childPrefix+g.th.Dim(label))
		*rows++
	}
}

func age(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
	return t.Format("2006-01-02")
}

// ---- live usage ---------------------------------------------------------

func printUsage(w io.Writer, s *sysinfo.Snapshot) {
	th := g.th
	host := s.Hostname
	if s.Platform != "" {
		host += " · " + s.Platform
	}
	fmt.Fprintf(w, "%s %s · up %s\n", th.Dim("host:"), host, units.HumanDuration(time.Duration(s.Uptime)*time.Second))
	cpuModel := s.CPU.Model
	if cpuModel == "" {
		cpuModel = "CPU"
	}
	load := ""
	if s.CPU.Load.L1 > 0 || s.CPU.Load.L5 > 0 {
		load = fmt.Sprintf(" · load %.2f %.2f %.2f", s.CPU.Load.L1, s.CPU.Load.L5, s.CPU.Load.L15)
	}
	fmt.Fprintf(w, "%s  %s · %d cores · %s%s\n", th.Dim("cpu: "), cpuModel, s.CPU.Cores, th.Bold(units.Percent(s.CPU.Percent, 1)), load)
	fmt.Fprintf(w, "      %s\n", style.Bar(s.CPU.Percent/100, 30, th))

	fmt.Fprintf(w, "%s  %s / %s · %s · available %s\n", th.Dim("mem: "),
		th.Bold(units.HumanBytes(int64(s.Mem.Used))),
		units.HumanBytes(int64(s.Mem.Total)),
		units.Percent(s.Mem.Percent, 1),
		units.HumanBytes(int64(s.Mem.Available)))
	fmt.Fprintf(w, "      %s\n", style.Bar(s.Mem.Percent/100, 30, th))
	if s.Mem.SwapTotal > 0 {
		fmt.Fprintf(w, "%s %s / %s · %s\n", th.Dim("swap:"),
			units.HumanBytes(int64(s.Mem.SwapUsed)),
			units.HumanBytes(int64(s.Mem.SwapTotal)),
			units.Percent(s.Mem.SwapPercent, 1))
	}

	if len(s.Disks) > 0 {
		fmt.Fprintf(w, "%s\n", th.Dim("disks:"))
		for _, d := range s.Disks {
			fmt.Fprintf(w, "      %s %s  %s free of %s  %s\n",
				style.Pad(style.Truncate(d.Mountpoint, 22), 22),
				style.Bar(d.Percent/100, 20, th),
				th.Bold(units.HumanBytes(int64(d.Free))),
				units.HumanBytes(int64(d.Total)),
				th.Dim(units.Percent(d.Percent, 0)+" used"),
			)
		}
	}

	if len(s.Net) > 0 {
		parts := make([]string, 0, len(s.Net))
		for _, n := range s.Net {
			if n.SendRate == 0 && n.RecvRate == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s ↓%s ↑%s", n.Name, units.HumanRate(n.RecvRate), units.HumanRate(n.SendRate)))
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "%s  %s\n", th.Dim("net: "), strings.Join(parts, "   "))
		}
	}

	if len(s.Procs) > 0 {
		fmt.Fprintf(w, "\n%s %s\n", th.Dim("processes:"), th.Dim(fmt.Sprintf("%d running", s.ProcCount)))
		rows := make([][]string, 0, len(s.Procs))
		for _, p := range s.Procs {
			rows = append(rows, []string{
				itoa(int(p.PID)),
				units.Percent(p.CPU, 1),
				units.HumanBytes(int64(p.RSS)),
				p.Name,
			})
		}
		t := &style.Table{
			Headers:  []string{"PID", "CPU", "MEM", "NAME"},
			Rows:     rows,
			Align:    []style.Align{style.Right, style.Right, style.Right, style.Left},
			MaxWidth: []int{8, 8, 10, 60},
		}
		fmt.Fprint(w, t.Render(th))
	}
}

// ---- cleanup tables -----------------------------------------------------

// ruleAgg aggregates every discovered target of one cleanup rule.
type ruleAgg struct {
	rule    clean.Rule
	size    int64
	files   int64
	targets []clean.Target
}

// printCleanRules prints the measured cleanup targets grouped by rule.
func printCleanRules(w io.Writer, targets []clean.Target, categories []string) int64 {
	th := g.th
	byRule := map[string]*ruleAgg{}
	var order []string
	for _, t := range targets {
		a, ok := byRule[t.Rule.ID]
		if !ok {
			a = &ruleAgg{rule: t.Rule}
			byRule[t.Rule.ID] = a
			order = append(order, t.Rule.ID)
		}
		a.size += t.Size
		a.files += t.Files
		a.targets = append(a.targets, t)
	}
	byCat := map[string][]*ruleAgg{}
	for _, id := range order {
		a := byRule[id]
		byCat[a.rule.Category] = append(byCat[a.rule.Category], a)
	}
	if len(categories) == 0 {
		for c := range byCat {
			categories = append(categories, c)
		}
		sort.Strings(categories)
	}
	var total int64
	for _, cat := range categories {
		items := byCat[cat]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].size > items[j].size })
		var catTotal int64
		rows := make([][]string, 0, len(items))
		for _, a := range items {
			catTotal += a.size
			risk := a.rule.Risk.String()
			switch a.rule.Risk {
			case clean.RiskHigh:
				risk = th.Red("high")
			case clean.RiskMedium:
				risk = th.Yellow("medium")
			default:
				risk = th.Green("low")
			}
			what := a.rule.Name
			if a.rule.Admin {
				what += th.Dim(" (admin)")
			}
			rows = append(rows, []string{
				colorShare(th, units.HumanBytes(a.size), share(a.size, totalCandidateTotal(targets))),
				risk,
				a.rule.ID,
				what,
				units.HumanCount(int64(len(a.targets))),
			})
		}
		fmt.Fprintf(w, "%s %s\n", th.Title(prettyCategory(cat)), th.Dim(units.HumanBytes(catTotal)))
		tb := &style.Table{
			Headers:  []string{"SIZE", "RISK", "ID", "WHAT", "PATHS"},
			Rows:     rows,
			Align:    []style.Align{style.Right, style.Left, style.Left, style.Left, style.Right},
			MaxWidth: []int{10, 8, 26, 46, 6},
		}
		fmt.Fprint(w, tb.Render(th))
		if g.verbose {
			for _, a := range items {
				sorted := append([]clean.Target(nil), a.targets...)
				sort.Slice(sorted, func(i, j int) bool { return sorted[i].Size > sorted[j].Size })
				for i, t := range sorted {
					if i >= 5 {
						fmt.Fprintf(w, "        %s\n", th.Dim(fmt.Sprintf("… %d more", len(sorted)-5)))
						break
					}
					fmt.Fprintf(w, "        %10s  %s\n", units.HumanBytes(t.Size), shortenPath(t.Path))
				}
			}
		}
		fmt.Fprintln(w)
		total += catTotal
	}
	return total
}

func totalCandidateTotal(targets []clean.Target) int64 { return clean.Total(targets) }

func prettyCategory(c string) string {
	switch c {
	case "dev":
		return "Development"
	case "browsers":
		return "Browsers"
	case "apps":
		return "Application caches"
	case "logs":
		return "Logs & reports"
	case "system":
		return "System (needs admin rights)"
	case "temp":
		return "Temporary files"
	}
	if c == "" {
		return "Other"
	}
	return strings.ToUpper(c[:1]) + c[1:]
}

func printCleanReport(w io.Writer, rep *clean.Report) {
	th := g.th
	verb := "Freed"
	if rep.DryRun {
		verb = "Would free"
	}
	fmt.Fprintf(w, "\n%s %s in %s across %s\n",
		th.Green(verb+":"), th.Bold(units.HumanBytes(rep.Bytes)), plural(rep.Files, "file", "files"),
		plural(int64(len(rep.Removed)), "target", "targets"))
	if len(rep.Failed) > 0 {
		fmt.Fprintf(w, "%s %d targets could not be removed\n", th.Yellow("warning:"), len(rep.Failed))
		for i, f := range rep.Failed {
			if i >= 10 {
				fmt.Fprintf(w, "  … and %d more\n", len(rep.Failed)-10)
				break
			}
			fmt.Fprintf(w, "  %s: %s\n", f.Path, th.Dim(f.Err))
		}
	}
	if rep.DryRun {
		fmt.Fprintf(w, "%s\n", th.Dim("This was a dry run. Re-run with --yes to delete for real."))
	}
}

// ---- trash --------------------------------------------------------------

func printTrash(w io.Writer, rep *trash.Report) {
	th := g.th
	fmt.Fprintf(w, "%s %s in %s items\n", th.Dim("trash:"), th.Bold(units.HumanBytes(rep.Bytes)), units.HumanCount(rep.Items))
	rows := make([][]string, 0, len(rep.Locations))
	blocked := false
	for _, l := range rep.Locations {
		status := th.Green("ok")
		if l.Err != "" {
			status = th.Dim(l.Err)
			if isPermissionErr(l.Err) {
				status = th.Yellow("no access")
				blocked = true
			}
		}
		rows = append(rows, []string{units.HumanBytes(l.Bytes), units.HumanCount(l.Items), l.Path, status})
	}
	t := &style.Table{
		Headers:  []string{"SIZE", "ITEMS", "LOCATION", "STATUS"},
		Rows:     rows,
		Align:    []style.Align{style.Right, style.Right, style.Left, style.Left},
		MaxWidth: []int{10, 8, 66, 18},
	}
	fmt.Fprint(w, t.Render(th))
	if blocked {
		fmt.Fprintf(w, "\n%s\n", th.Dim("macOS protects the trash: grant your terminal “Full Disk Access” in System Settings →\n"+
			"Privacy & Security to let vynsh measure and empty it."))
	}
}

// isPermissionErr reports whether the error means the process is not allowed to
// read or write the path.
func isPermissionErr(s string) bool {
	for _, marker := range []string{"permission denied", "operation not permitted", "access is denied"} {
		if strings.Contains(strings.ToLower(s), marker) {
			return true
		}
	}
	return false
}

// plural renders "1 file" / "3 files".
func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return units.HumanCount(n) + " " + many
}

// ---- selective deletion -------------------------------------------------

func printPlan(w io.Writer, plan *del.Plan) {
	th := g.th
	rows := make([][]string, 0, len(plan.Items))
	for _, it := range plan.Items {
		kind := "file"
		if it.IsDir {
			kind = "dir "
		}
		rows = append(rows, []string{units.HumanBytes(it.Size), kind, age(it.ModTime), it.Path})
	}
	t := &style.Table{
		Headers:  []string{"SIZE", "KIND", "MODIFIED", "PATH"},
		Rows:     rows,
		Align:    []style.Align{style.Right, style.Left, style.Right, style.Left},
		MaxWidth: []int{10, 4, 12, 90},
	}
	if len(rows) > maxConfirmItems {
		t.Rows = rows[:maxConfirmItems]
	}
	fmt.Fprint(w, t.Render(th))
	if len(rows) > maxConfirmItems {
		fmt.Fprintf(w, "%s\n", th.Dim(fmt.Sprintf("… and %d more", len(rows)-maxConfirmItems)))
	}
	fmt.Fprintf(w, "%s %s in %s", th.Dim("selection:"), th.Bold(units.HumanBytes(plan.Bytes)), plural(plan.Files, "file", "files"))
	if plan.Dirs > 0 {
		fmt.Fprintf(w, " + %s", plural(plan.Dirs, "empty dir", "empty dirs"))
	}
	if plan.Skipped > 0 {
		fmt.Fprintf(w, " %s", th.Dim(fmt.Sprintf("(%s did not match)", plural(plan.Skipped, "entry", "entries"))))
	}
	fmt.Fprintln(w)
	if len(plan.Errors) > 0 && g.verbose {
		for _, e := range plan.Errors {
			fmt.Fprintf(w, "  %s\n", th.Dim(e))
		}
	}
}

// ---- duplicates ---------------------------------------------------------

func printDupes(w io.Writer, res *dupes.Result) {
	th := g.th
	fmt.Fprintf(w, "%s %d files scanned · %d candidates · %d duplicate groups · %s recoverable · %.1fs\n\n",
		th.Dim("duplicates:"), res.FilesScanned, res.Candidates, len(res.Groups),
		th.Bold(units.HumanBytes(res.Wasted)), res.Elapsed)
	for i, grp := range res.Groups {
		if i >= 30 {
			fmt.Fprintf(w, "%s\n", th.Dim(fmt.Sprintf("… and %d more groups", len(res.Groups)-30)))
			break
		}
		fmt.Fprintf(w, "%s  %s  %s\n",
			th.Bold(units.HumanBytes(grp.Size)),
			th.Dim(fmt.Sprintf("×%d", len(grp.Paths))),
			th.Dim(fmt.Sprintf("(save %s)", units.HumanBytes(grp.Wasted()))))
		for _, p := range grp.Paths {
			fmt.Fprintf(w, "    %s\n", shortenPath(p))
		}
		fmt.Fprintln(w)
	}
}

func shortenPath(p string) string {
	if hd, err := userHome(); err == nil && strings.HasPrefix(p, hd) {
		return "~" + p[len(hd):]
	}
	return p
}

func userHome() (string, error) {
	return os.UserHomeDir()
}

// ---- misc ---------------------------------------------------------------

// printSpaceLine reports how the free space changed after a cleanup.
func printSpaceLine(w io.Writer, before, after uint64) {
	th := g.th
	if before == 0 || after == 0 {
		return
	}
	if after <= before {
		fmt.Fprintf(w, "%s free space: %s %s\n", th.Dim("disk"), units.HumanBytes(int64(before)),
			th.Dim("(unchanged — the OS may free the space later)"))
		return
	}
	fmt.Fprintf(w, "%s free space: %s → %s %s\n", th.Green("✓"),
		units.HumanBytes(int64(before)), th.Bold(units.HumanBytes(int64(after))),
		th.Green("(+"+units.HumanBytes(int64(after-before))+")"))
}
