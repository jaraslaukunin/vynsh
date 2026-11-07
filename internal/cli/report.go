package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// cacheLine is one row of the "reclaimable" section.
type cacheLine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Risk  string `json:"risk"`
}

// reportData is the JSON shape of `vynsh report`.
type reportData struct {
	Generated   time.Time          `json:"generated"`
	Host        string             `json:"host"`
	Platform    string             `json:"platform"`
	Disks       []sysinfo.DiskInfo `json:"disks"`
	Mem         sysinfo.MemInfo    `json:"memory"`
	CPU         sysinfo.CPUInfo    `json:"cpu"`
	Home        string             `json:"home"`
	HomeTotal   int64              `json:"home_total"`
	HomeTop     []scan.Node        `json:"home_top"`
	HomeFiles   int64              `json:"home_files"`
	Caches      []cacheLine        `json:"caches"`
	CacheTotal  int64              `json:"cache_total"`
	TrashBytes  int64              `json:"trash_bytes"`
	TrashItems  int64              `json:"trash_items"`
	Reclaimable int64              `json:"reclaimable"`
	Suggestions []string           `json:"suggestions"`
}

func newReportCmd() *cobra.Command {
	var (
		depth    int
		skipScan bool
		topN     int
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "One-screen health report with reclaimable space",
		Long: `Combine everything vynsh knows into a single overview: volume usage, memory
pressure, the biggest directories in your home folder, how much junk is
reclaimable and what to do next.

Examples:
  vynsh report
  vynsh report --depth 2 --top 10
  vynsh report --skip-scan        # fast: skip the home directory walk
  vynsh report --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if depth <= 0 {
				depth = 1
			}
			if topN <= 0 {
				topN = 10
			}

			sp := style.NewSpinner(os.Stderr, g.th, "collecting usage")
			sp.Start()
			hostInfo := sysinfo.NewMonitor(sysinfo.Options{Interval: 120 * time.Millisecond, IncludeProcs: false})
			snap, snapErr := hostInfo.Collect()
			if snapErr != nil {
				sp.Stop()
				return snapErr
			}
			sp.Update("measuring caches")
			data := reportData{Generated: time.Now(), Mem: snap.Mem, CPU: snap.CPU, Disks: snap.Disks}
			data.Host = snap.Hostname
			data.Platform = snap.Platform

			rules, _ := clean.Select(nil, nil, false, false)
			targets := clean.Discover(rules, clean.DiscoverOptions{})
			byRule := map[string]*cacheLine{}
			for _, t := range targets {
				if !t.Exists || t.Size == 0 {
					continue
				}
				if line, ok := byRule[t.Rule.ID]; ok {
					line.Bytes += t.Size
					continue
				}
				byRule[t.Rule.ID] = &cacheLine{ID: t.Rule.ID, Name: t.Rule.Name, Bytes: t.Size, Risk: t.Rule.Risk.String()}
			}
			for _, line := range byRule {
				data.Caches = append(data.Caches, *line)
				data.CacheTotal += line.Bytes
			}
			sort.Slice(data.Caches, func(i, j int) bool { return data.Caches[i].Bytes > data.Caches[j].Bytes })

			if trash.Supported() {
				if tr, err := trash.Inspect(); err == nil && tr != nil {
					data.TrashBytes = tr.Bytes
					data.TrashItems = tr.Items
				}
			}

			home := fsutil.HomeDir()
			data.Home = home
			if !skipScan && home != "" {
				sp.Update("scanning " + shortenPath(home))
				res, err := scan.Run(scan.Options{Root: home, Depth: depth, TopFiles: 5, Progress: func(files int64, bytes int64) {
					sp.Update(fmt.Sprintf("scanning %s — %s files, %s", shortenPath(home), units.HumanCount(files), units.HumanBytes(bytes)))
				}})
				if err == nil {
					data.HomeTotal = res.Total
					data.HomeFiles = res.Files
					top := append([]*scan.Node(nil), res.Tree.Children...)
					sort.Slice(top, func(i, j int) bool { return top[i].Size > top[j].Size })
					if len(top) > topN {
						top = top[:topN]
					}
					data.HomeTop = make([]scan.Node, 0, len(top))
					for _, n := range top {
						data.HomeTop = append(data.HomeTop, *n)
					}
				}
			}
			sp.Stop()

			data.Reclaimable = data.CacheTotal + data.TrashBytes
			data.Suggestions = buildSuggestions(&data)

			if g.json {
				return mustJSON(data)
			}
			printReport(out, &data)
			return nil
		},
	}
	cmd.Flags().IntVarP(&depth, "depth", "d", 1, "how deep to analyse the home directory")
	cmd.Flags().IntVarP(&topN, "top", "n", 10, "how many big directories to list")
	cmd.Flags().BoolVar(&skipScan, "skip-scan", false, "skip walking the home directory (much faster)")
	return cmd
}

func printReport(w io.Writer, d *reportData) {
	th := g.th
	fmt.Fprintf(w, "%s %s\n", th.Title("vynsh report"), th.Dim(d.Generated.Format("2006-01-02 15:04")))
	fmt.Fprintf(w, "%s %s · %s · %s\n\n", th.Dim("host:"), d.Host, d.Platform, osName())

	for _, disk := range d.Disks {
		used := 100 - share(int64(disk.Free), int64(disk.Total))
		fmt.Fprintf(w, "%s %s %s  %s used of %s · %s free\n", th.Dim("disk:"),
			style.Pad(disk.Mountpoint, 16), style.Bar(disk.Percent/100, 20, th),
			th.Bold(units.Percent(used, 0)), units.HumanBytes(int64(disk.Total)),
			th.Bold(units.HumanBytes(int64(disk.Free))))
	}
	fmt.Fprintf(w, "%s %s / %s · %s\n", th.Dim("mem: "),
		units.HumanBytes(int64(d.Mem.Used)), units.HumanBytes(int64(d.Mem.Total)),
		units.Percent(d.Mem.Percent, 1))

	if d.HomeTotal > 0 {
		fmt.Fprintf(w, "\n%s %s · %s · %s files\n", th.Title("Home"), shortenPath(d.Home),
			th.Bold(units.HumanBytes(d.HomeTotal)), units.HumanCount(d.HomeFiles))
		rows := make([][]string, 0, len(d.HomeTop))
		for _, n := range d.HomeTop {
			pct := share(n.Size, d.HomeTotal)
			rows = append(rows, []string{
				units.HumanBytes(n.Size),
				units.Percent(pct, 1),
				style.Bar(pct/100, 16, th),
				n.Name,
			})
		}
		t := &style.Table{
			Headers:  []string{"SIZE", "SHARE", "", "DIRECTORY"},
			Rows:     rows,
			Align:    []style.Align{style.Right, style.Right, style.Left, style.Left},
			MaxWidth: []int{10, 7, 18, 50},
		}
		fmt.Fprint(w, t.Render(th))
	}

	fmt.Fprintf(w, "\n%s %s\n", th.Title("Reclaimable"), th.Bold(units.HumanBytes(d.Reclaimable)))
	if len(d.Caches) > 0 {
		rows := make([][]string, 0, len(d.Caches))
		for i, c := range d.Caches {
			if i >= 8 {
				break
			}
			rows = append(rows, []string{units.HumanBytes(c.Bytes), c.Name, th.Dim(c.ID)})
		}
		t := &style.Table{
			Headers:  []string{"SIZE", "WHAT", "RULE"},
			Rows:     rows,
			Align:    []style.Align{style.Right, style.Left, style.Left},
			MaxWidth: []int{10, 40, 24},
		}
		fmt.Fprint(w, t.Render(th))
	}
	if d.TrashBytes > 0 {
		fmt.Fprintf(w, "%10s  %s (%s items)\n", units.HumanBytes(d.TrashBytes), "Trash", units.HumanCount(d.TrashItems))
	}
	if len(d.Suggestions) > 0 {
		fmt.Fprintf(w, "\n%s\n", th.Title("What to do next"))
		for _, s := range d.Suggestions {
			fmt.Fprintf(w, "  %s %s\n", th.Cyan("•"), s)
		}
	}
}

// buildSuggestions derives concrete next steps from the collected data.
func buildSuggestions(d *reportData) []string {
	var out []string
	if d.CacheTotal > 200_000_000 {
		out = append(out, fmt.Sprintf("vynsh clean              # removes %s of caches", units.HumanBytes(d.CacheTotal)))
	}
	if d.TrashBytes > 100_000_000 {
		out = append(out, fmt.Sprintf("vynsh trash empty        # removes %s from the trash", units.HumanBytes(d.TrashBytes)))
	}
	var biggest *scan.Node
	for i := range d.HomeTop {
		if biggest == nil || d.HomeTop[i].Size > biggest.Size {
			biggest = &d.HomeTop[i]
		}
	}
	if biggest != nil && d.HomeTotal > 0 && share(biggest.Size, d.HomeTotal) > 15 {
		out = append(out, fmt.Sprintf("vynsh scan %s --depth 2   # %s is %s of your home",
			shortenPath(biggest.Path), biggest.Name, units.Percent(share(biggest.Size, d.HomeTotal), 0)))
	}
	for _, disk := range d.Disks {
		freePct := share(int64(disk.Free), int64(disk.Total))
		if disk.Mountpoint == "/" || (len(disk.Mountpoint) == 3 && disk.Mountpoint[1] == ':') {
			if freePct < 15 {
				out = append(out, fmt.Sprintf("%s is %s full — free space soon", disk.Mountpoint, units.Percent(100-freePct, 0)))
			}
		}
	}
	if d.Mem.Percent > 85 {
		out = append(out, "vynsh usage --sort mem   # memory is tight, see what is using it")
	}
	if d.HomeTotal > 0 && d.Reclaimable == 0 {
		out = append(out, "vynsh dupes ~           # nothing to clean — look for duplicate files")
	}
	if len(out) == 0 {
		out = append(out, "Nothing urgent. Run `vynsh scan` on a large folder to dig deeper.")
	}
	return out
}
