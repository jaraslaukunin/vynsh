package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/tui"
)

func newTUICmd() *cobra.Command {
	var refresh time.Duration
	cmd := &cobra.Command{
		Use:     "tui",
		Aliases: []string{"dash", "dashboard"},
		Short:   "Open the interactive dashboard",
		Long: `Open a live dashboard in the terminal: resource overview, an interactive
cleanup picker, disk analysis and the trash — all in one screen.

Views
  1 Overview   CPU, memory, disks, network, busiest processes
  2 Cleanup    tick rules with space, see what each one frees, then apply
  3 Disk       scan your home folder and save a snapshot for later comparison
  4 Trash      what is in the bin, restore items vynsh moved there, empty it

Keys
  1-4 / tab      switch views (←/→ work too)
  ↑↓ space       move and tick cleanup rules
  a / n          select all safe rules / select none
  enter          apply the selection or empty the trash (asks first)
  u              restore items from the trash
  s / v          scan the home folder / save the scan as a snapshot
  r              refresh everything · q quit`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.json {
				return fmt.Errorf("--json is not supported by the interactive dashboard; use `vynsh usage --json`")
			}
			return tui.Run(tui.Options{Refresh: refresh, Version: shortVersion()})
		},
	}
	cmd.Flags().DurationVar(&refresh, "refresh", 2*time.Second, "how often to refresh the snapshot")
	return cmd
}

// shortVersion turns "0.3.0-beta" into "0.3 beta" for the dashboard header.
func shortVersion() string {
	v := strings.TrimSuffix(Version, "-dev")
	parts := strings.SplitN(v, "-", 2)
	base := parts[0]
	nums := strings.Split(base, ".")
	if len(nums) >= 2 {
		base = nums[0] + "." + nums[1]
	}
	if len(parts) == 2 && parts[1] != "" {
		return base + " " + parts[1]
	}
	return base
}
