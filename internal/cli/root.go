// Package cli wires the vynsh commands together with spf13/cobra.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/config"
	"github.com/jaraslaukunin/vynsh/internal/style"
)

// Build metadata, injected with -ldflags at release time.
var (
	Version = "0.3.0-beta"
	Commit  = "none"
	Date    = "unknown"
)

// globals holds flags shared by every command.
type globals struct {
	json    bool
	noColor bool
	yes     bool
	verbose bool
	cfg     *config.Config
	th      *style.Theme
}

var g globals

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "vynsh",
		Short: "Clean up your disk, trash and processes — on macOS and Windows",
		Long: strings.TrimSpace(`
vynsh (винш) — a fast, cross-platform cleanup and analysis CLI.

It finds out what actually fills your disk, removes the junk that is safe to
remove, and shows what your computer is busy with. Destructive commands always
print a plan first: they ask for confirmation in a terminal, stay read-only in
scripts unless you pass --yes, and honour --dry-run everywhere.

Typical first run:

  vynsh report                 # what is big, what is junk, how much can be freed
  vynsh scan ~/Downloads       # where the space went
  vynsh clean                  # measure junk, then confirm removal
  vynsh usage                  # CPU / memory / disk / network live snapshot
  vynsh tui                    # interactive dashboard with the mascot
  vynsh snapshot save ~        # remember disk usage, diff it later`),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err == nil {
				cfg.Apply()
				g.cfg = cfg
			} else {
				g.cfg = config.Default()
				if g.verbose {
					fmt.Fprintf(os.Stderr, "vynsh: config: %v\n", err)
				}
			}
			if g.noColor || g.cfg.NoColor || os.Getenv("NO_COLOR") != "" {
				g.th = &style.Theme{}
			} else {
				g.th = style.Default()
			}
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&g.json, "json", false, "output machine readable JSON")
	root.PersistentFlags().BoolVar(&g.noColor, "no-color", false, "disable ANSI colours")
	root.PersistentFlags().BoolVarP(&g.yes, "yes", "y", false, "do not ask for confirmation")
	root.PersistentFlags().BoolVarP(&g.verbose, "verbose", "v", false, "explain what is happening")

	root.AddGroup(
		&cobra.Group{ID: "analyse", Title: "Analysis:"},
		&cobra.Group{ID: "cleanup", Title: "Cleanup:"},
		&cobra.Group{ID: "tools", Title: "Other:"},
	)

	for _, c := range []*cobra.Command{
		newScanCmd(), newUsageCmd(), newDupesCmd(), newReportCmd(), newSnapshotCmd(),
	} {
		c.GroupID = "analyse"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{
		newCleanCmd(), newTrashCmd(), newRmCmd(), newDockerCmd(),
	} {
		c.GroupID = "cleanup"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{
		newTUICmd(), newAuditCmd(), newConfigCmd(), newLogoCommand(), newVersionCmd(),
	} {
		c.GroupID = "tools"
		root.AddCommand(c)
	}
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(c *cobra.Command, args []string) error {
			cmd, _, err := c.Root().Find(args)
			if cmd == nil || err != nil {
				return fmt.Errorf("unknown command %q", strings.Join(args, " "))
			}
			return cmd.Help()
		},
	})
	return root
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	if err := NewRoot().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Default().Red("error:"), err)
		return 1
	}
	return 0
}
