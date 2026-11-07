package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/sysinfo"
)

func newUsageCmd() *cobra.Command {
	var (
		interval time.Duration
		topN     int
		sortBy   string
		watch    bool
		noProcs  bool
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show live CPU, memory, disk, network and process usage",
		Long: `Take a resource usage snapshot and list the busiest processes.

With --watch the screen is refreshed until you press Ctrl+C. Process CPU is a
delta between two samples, so the first snapshot is always accurate (vynsh
samples internally for --interval).

Examples:
  vynsh usage
  vynsh usage --watch
  vynsh usage --sort mem --top 15
  vynsh usage --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				interval = 300 * time.Millisecond
			}
			if topN <= 0 {
				topN = 10
			}
			switch sortBy {
			case "cpu", "mem":
			default:
				return fmt.Errorf("--sort must be cpu or mem")
			}
			mon := sysinfo.NewMonitor(sysinfo.Options{
				Interval:     interval,
				TopN:         topN,
				SortBy:       sortBy,
				IncludeProcs: !noProcs,
			})
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			out := cmd.OutOrStdout()
			for {
				snap, err := mon.Collect()
				if err != nil {
					return err
				}
				if g.json {
					if err := mustJSON(snap); err != nil {
						return err
					}
				} else {
					if watch {
						fmt.Fprint(out, "\x1b[2J\x1b[H")
						fmt.Fprintf(out, "%s %s\n", g.th.Dim("vynsh usage —"), g.th.Dim(time.Now().Format("15:04:05")))
					}
					printUsage(out, snap)
					if watch {
						fmt.Fprintf(out, "\n%s\n", g.th.Dim("refreshing every second — press Ctrl+C to stop"))
					}
				}
				if !watch {
					return nil
				}
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(time.Second):
				}
			}
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 300*time.Millisecond, "CPU sampling window")
	cmd.Flags().IntVarP(&topN, "top", "n", 10, "how many processes to list")
	cmd.Flags().StringVar(&sortBy, "sort", "cpu", "sort processes by cpu or mem")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh continuously")
	cmd.Flags().BoolVar(&noProcs, "no-processes", false, "skip the process list (faster)")
	return cmd
}
