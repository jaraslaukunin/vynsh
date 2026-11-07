package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newScanCmd() *cobra.Command {
	var (
		depth    int
		top      int
		minSize  string
		excludes []string
		bars     bool
		all      bool
		noBar    bool
		oneFS    bool
	)
	cmd := &cobra.Command{
		Use:   "scan [path ...]",
		Short: "Analyse disk usage: biggest directories and files",
		Long: `Walk one or more directories and report where the space went.

Output shows total size, a directory tree with sizes and shares, the largest
files and a breakdown by file type. Symbolic links are never followed, hidden
system trees such as /System/Volumes (macOS) are skipped, and unreadable paths
are reported instead of aborting the walk.

Examples:
  vynsh scan                     # current directory
  vynsh scan ~/Downloads -d 3    # three levels deep
  vynsh scan / --all             # whole disk, full tree
  vynsh scan . --min-size 100MB  # hide anything smaller than 100 MB`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				args = []string{"."}
			}
			useBars := bars && !noBar && g.th.Enabled
			minBytes := int64(0)
			if minSize != "" {
				v, err := units.ParseBytes(minSize)
				if err != nil {
					return err
				}
				minBytes = v
			}
			topFiles := top
			if topFiles <= 0 {
				topFiles = g.cfg.Top
			}
			if depth <= 0 {
				depth = g.cfg.ScanDepth
			}
			var results []*scan.Result
			for _, arg := range args {
				res, err := scanOne(arg, depth, topFiles, minBytes, excludes)
				if err != nil {
					return err
				}
				results = append(results, res)
			}
			if g.json {
				if len(results) == 1 {
					return mustJSON(results[0])
				}
				return mustJSON(results)
			}
			for i, res := range results {
				if i > 0 {
					fmt.Fprintln(out)
				}
				opts := treeOptions{Bars: useBars, MinSize: autoMin(res, minBytes), MaxRows: 40, MaxDepth: depth}
				if all {
					opts.MaxRows = 1 << 20
					opts.MinSize = 0
				}
				printScan(out, res, opts)
			}
			if len(args) == 1 && !all {
				fmt.Fprintf(out, "%s\n", g.th.Dim("go deeper:  vynsh scan "+args[0]+" --depth "+
					fmt.Sprint(depth+1)+" --all     (hides nothing, shows full tree)"))
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&depth, "depth", "d", 0, "directory tree depth to show (default from config, usually 2)")
	cmd.Flags().IntVarP(&top, "top", "n", 0, "number of largest files to list")
	cmd.Flags().StringVar(&minSize, "min-size", "", "hide entries smaller than this (e.g. 100MB)")
	cmd.Flags().StringSliceVar(&excludes, "exclude", nil, "paths or glob patterns to skip (repeatable)")
	cmd.Flags().BoolVar(&bars, "bars", true, "draw proportion bars")
	cmd.Flags().BoolVar(&noBar, "no-bars", false, "do not draw proportion bars")
	cmd.Flags().BoolVar(&all, "all", false, "show every entry, however small")
	cmd.Flags().BoolVar(&oneFS, "one-filesystem", false, "stay on the filesystem of each root")
	return cmd
}

// autoMin computes a sensible display threshold (0.5% of the total) unless the
// user asked for a specific one.
func autoMin(res *scan.Result, explicit int64) int64 {
	if explicit > 0 {
		return explicit
	}
	if res.Total < 10 {
		return 0
	}
	return res.Total / 200
}

// scanOne runs a scan with a progress spinner.
func scanOne(path string, depth, topFiles int, minSize int64, excludes []string) (*scan.Result, error) {
	expanded := fsutil.ExpandPath(path)
	sp := style.NewSpinner(os.Stderr, g.th, "scanning "+shortenPath(expanded))
	sp.Start()
	var lastUpdate time.Time
	res, err := scan.Run(scan.Options{
		Root:     expanded,
		Depth:    depth,
		TopFiles: topFiles,
		TopDirs:  maxInt(topFiles, 15),
		MinSize:  minSize,
		Exclude:  append(excludes, g.cfg.Excludes...),
		Progress: func(files int64, bytes int64) {
			if time.Since(lastUpdate) < 100*time.Millisecond {
				return
			}
			lastUpdate = time.Now()
			sp.Update(fmt.Sprintf("scanning %s — %s files, %s", shortenPath(expanded), units.HumanCount(files), units.HumanBytes(bytes)))
		},
	})
	sp.Stop()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
