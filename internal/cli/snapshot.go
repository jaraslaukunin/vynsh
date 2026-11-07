package cli

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/snapshot"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Save disk usage snapshots and see what grew since last time",
		Long: `Save the result of a disk scan, then compare a later state against it.

A snapshot is a scanned directory tree stored as JSON, so "what ate my disk this
week?" can be answered instantly: the old tree is compared with a fresh scan (or
with another stored snapshot).

Examples:
  vynsh snapshot save ~ --depth 2 --name before-update
  vynsh snapshot list
  vynsh snapshot diff before-update            # compare with a fresh scan
  vynsh snapshot diff before-update before-update2
  vynsh snapshot show before-update`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listSnapshots(cmd.OutOrStdout())
		},
	}
	cmd.AddCommand(newSnapshotSaveCmd(), newSnapshotListCmd(), newSnapshotShowCmd(),
		newSnapshotDiffCmd(), newSnapshotDeleteCmd())
	return cmd
}

func newSnapshotSaveCmd() *cobra.Command {
	var (
		depth int
		name  string
	)
	cmd := &cobra.Command{
		Use:   "save [path]",
		Short: "Scan a directory and store the result",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			root := "."
			if len(args) > 0 {
				root = args[0]
			}
			if depth <= 0 {
				depth = 2
			}
			res, err := scanOne(root, depth, 15, 0, nil)
			if err != nil {
				return err
			}
			snap, err := snapshot.Save(name, res, depth)
			if err != nil {
				return err
			}
			if _, err := snapshot.Prune(50); err != nil && g.verbose {
				fmt.Fprintf(out, "%s pruning old snapshots: %v\n", g.th.Dim("note:"), err)
			}
			if g.json {
				return mustJSON(snap.Meta)
			}
			fmt.Fprintf(out, "%s snapshot %s saved\n", g.th.Green("✓"), g.th.Bold(snap.Name))
			fmt.Fprintf(out, "  root:  %s\n", snap.Root)
			fmt.Fprintf(out, "  size:  %s · %s files · depth %d\n",
				units.HumanBytes(snap.Total), units.HumanCount(snap.Files), snap.Depth)
			fmt.Fprintf(out, "  file:  %s\n", mustPath(snap.Name))
			fmt.Fprintf(out, "\n%s\n", g.th.Dim("compare later: vynsh snapshot diff "+snap.Name))
			return nil
		},
	}
	cmd.Flags().IntVarP(&depth, "depth", "d", 0, "directory depth to record (default 2)")
	cmd.Flags().StringVarP(&name, "name", "n", "", "snapshot name (default: <dir>-<date>)")
	return cmd
}

func mustPath(name string) string {
	p, err := snapshot.Path(name)
	if err != nil {
		return name
	}
	return p
}

func newSnapshotListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List stored snapshots",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listSnapshots(cmd.OutOrStdout())
		},
	}
}

func listSnapshots(w io.Writer) error {
	metas, err := snapshot.List()
	if err != nil {
		return err
	}
	if g.json {
		return mustJSON(metas)
	}
	if len(metas) == 0 {
		fmt.Fprintf(w, "%s\n", g.th.Dim("No snapshots yet. Create one with: vynsh snapshot save ~ --name before-cleanup"))
		return nil
	}
	rows := make([][]string, 0, len(metas))
	for _, m := range metas {
		rows = append(rows, []string{
			m.Name,
			m.Created.Format("2006-01-02 15:04"),
			units.HumanBytes(m.Total),
			units.HumanCount(m.Files),
			strconv.Itoa(m.Depth),
			shortenPath(m.Root),
		})
	}
	t := &style.Table{
		Headers:  []string{"NAME", "TAKEN", "SIZE", "FILES", "DEPTH", "ROOT"},
		Rows:     rows,
		Align:    []style.Align{style.Left, style.Left, style.Right, style.Right, style.Right, style.Left},
		MaxWidth: []int{34, 19, 10, 10, 6, 50},
	}
	fmt.Fprint(w, t.Render(g.th))
	fmt.Fprintf(w, "\n%s\n", g.th.Dim("diff: vynsh snapshot diff <name>"))
	return nil
}

func newSnapshotShowCmd() *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a stored snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := snapshot.Load(args[0])
			if err != nil {
				return err
			}
			if g.json {
				return mustJSON(snap)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n", g.th.Title("snapshot:"), g.th.Bold(snap.Name))
			fmt.Fprintf(out, "%s\n", g.th.Dim(fmt.Sprintf("taken %s · root %s · depth %d",
				snap.Created.Format("2006-01-02 15:04"), snap.Root, snap.Depth)))
			if snap.Result == nil {
				fmt.Fprintf(out, "%s\n", g.th.Dim("no scan data inside this snapshot"))
				return nil
			}
			if depth <= 0 {
				depth = snap.Depth
			}
			printScan(out, snap.Result, treeOptions{
				Bars:     g.th.Enabled,
				MinSize:  autoMin(snap.Result, 0),
				MaxRows:  40,
				MaxDepth: depth,
			})
			return nil
		},
	}
	cmd.Flags().IntVarP(&depth, "depth", "d", 0, "tree depth to print (default: as recorded)")
	return cmd
}

func newSnapshotDiffCmd() *cobra.Command {
	var (
		noScan   bool
		topN     int
		minDelta string
	)
	cmd := &cobra.Command{
		Use:   "diff <name> [name2]",
		Short: "Compare a snapshot with a fresh scan or with another snapshot",
		Long: `Compare snapshots directory by directory and show what grew or shrank.

With a single name the directory is scanned again right now (so you compare
"then" with "now"); pass a second name to compare two stored snapshots.

Examples:
  vynsh snapshot diff before-update
  vynsh snapshot diff monday friday --no-scan
  vynsh snapshot diff before-update --min-delta 100MB`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			a, err := snapshot.Load(args[0])
			if err != nil {
				return err
			}
			var b *snapshot.Snapshot
			switch {
			case len(args) == 2:
				b, err = snapshot.Load(args[1])
				if err != nil {
					return err
				}
			case noScan:
				meta, err := snapshot.LatestBefore(a.Name, a.Root)
				if err != nil {
					return err
				}
				b, err = snapshot.Load(meta.Name)
				if err != nil {
					return err
				}
			default:
				depth := a.Depth
				if depth <= 0 {
					depth = 2
				}
				res, err := scanOne(a.Root, depth, 15, 0, nil)
				if err != nil {
					return err
				}
				b = &snapshot.Snapshot{
					Meta: snapshot.Meta{
						Name: "now", Created: time.Now(), Root: res.Root,
						Depth: depth, Total: res.Total, Files: res.Files,
					},
					Result: res,
				}
			}
			diff := snapshot.Compare(a, b)
			if minDelta != "" {
				v, err := units.ParseBytes(minDelta)
				if err != nil {
					return err
				}
				filtered := diff.Changes[:0]
				for _, c := range diff.Changes {
					if absInt64(c.Delta) >= v {
						filtered = append(filtered, c)
					}
				}
				diff.Changes = filtered
			}
			if g.json {
				return mustJSON(diff)
			}
			printDiff(out, diff, topN)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noScan, "no-scan", false, "compare against the previous stored snapshot instead of scanning now")
	cmd.Flags().IntVarP(&topN, "top", "n", 20, "how many changes to show")
	cmd.Flags().StringVar(&minDelta, "min-delta", "", "hide changes smaller than this (e.g. 100MB)")
	return cmd
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func printDiff(w io.Writer, d *snapshot.Diff, topN int) {
	th := g.th
	fmt.Fprintf(w, "%s %s → %s\n", th.Title("snapshot diff"), th.Bold(d.A.Name), th.Bold(d.B.Name))
	fmt.Fprintf(w, "%s\n", th.Dim(fmt.Sprintf("%s (%s)  →  %s (%s)",
		d.A.Created.Format("2006-01-02 15:04"), units.HumanBytes(d.A.Total),
		d.B.Created.Format("2006-01-02 15:04"), units.HumanBytes(d.B.Total))))
	delta := d.TotalDelta
	sign := "+"
	colorize := th.Red
	if delta < 0 {
		sign = ""
		colorize = th.Green
	}
	filesText := "+" + plural(d.FilesDelta, "file", "files")
	if d.FilesDelta < 0 {
		filesText = "-" + plural(-d.FilesDelta, "file", "files")
	} else if d.FilesDelta == 0 {
		filesText = "no file count change"
	}
	fmt.Fprintf(w, "%s %s · %s\n\n", th.Dim("total:"),
		colorize(sign+units.HumanBytes(delta)), filesText)

	if len(d.Changes) == 0 {
		fmt.Fprintf(w, "%s\n", th.Dim("Nothing changed."))
		return
	}
	rows := make([][]string, 0, len(d.Changes))
	for i, c := range d.Changes {
		if topN > 0 && i >= topN {
			break
		}
		var deltaCell string
		switch {
		case c.Kind == snapshot.KindAdded && c.Before == 0:
			deltaCell = th.Yellow("new " + units.HumanBytes(c.Delta))
		case c.Kind == snapshot.KindRemoved && c.After == 0:
			deltaCell = th.Green("gone " + units.HumanBytes(c.Before))
		case c.Delta > 0:
			deltaCell = th.Red("+" + units.HumanBytes(c.Delta))
		default:
			deltaCell = th.Green("-" + units.HumanBytes(-c.Delta))
		}
		before, after := units.HumanBytes(c.Before), units.HumanBytes(c.After)
		if c.Kind == snapshot.KindAdded {
			before = "—"
		}
		if c.Kind == snapshot.KindRemoved {
			after = "—"
		}
		rows = append(rows, []string{deltaCell, before, after, c.Path})
	}
	t := &style.Table{
		Headers:  []string{"CHANGE", "BEFORE", "AFTER", "PATH"},
		Rows:     rows,
		Align:    []style.Align{style.Right, style.Right, style.Right, style.Left},
		MaxWidth: []int{16, 10, 10, 80},
	}
	fmt.Fprint(w, t.Render(th))
	if topN > 0 && len(d.Changes) > topN {
		fmt.Fprintf(w, "%s\n", th.Dim(fmt.Sprintf("… and %d more changes (raise --top to see them)", len(d.Changes)-topN)))
	}
}

func newSnapshotDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a stored snapshot",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := snapshot.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s snapshot %s deleted\n", g.th.Green("✓"), args[0])
			return nil
		},
	}
}
