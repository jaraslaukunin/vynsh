package cli

import (
	"fmt"
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
		Long: `Open a live dashboard in the terminal.

Views:
  1 Overview   CPU, memory, disks, network and the busiest processes
  2 Disk usage biggest directories of your home folder (press s to scan)
  3 Junk       reclaimable caches and the size of the trash

Keys: 1/2/3 or tab switch views · s scan home · r refresh · q quit`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.json {
				return fmt.Errorf("--json is not supported by the interactive dashboard; use `vynsh usage --json`")
			}
			return tui.Run(tui.Options{Refresh: refresh})
		},
	}
	cmd.Flags().DurationVar(&refresh, "refresh", 2*time.Second, "how often to refresh the snapshot")
	return cmd
}
