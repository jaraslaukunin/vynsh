package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/config"
	"github.com/jaraslaukunin/vynsh/internal/style"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.json {
				return writeJSON(map[string]string{
					"version": Version, "commit": Commit, "date": Date,
					"os": osName(), "config": config.Path(),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vynsh %s (%s, %s)\n", Version, Commit, Date)
			fmt.Fprintf(cmd.OutOrStdout(), "platform: %s\n", osName())
			fmt.Fprintf(cmd.OutOrStdout(), "config:   %s\n", config.Path())
			return nil
		},
	}
}

func newConfigCmd() *cobra.Command {
	var initOnly bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the configuration file and its location",
		Long: `Show the effective configuration.

The file is optional; vynsh works with built-in defaults. Use --init to create
it, then edit "excludes" to skip paths in every scan and "protected" to add
paths that must never be deleted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if initOnly {
				p, err := config.WriteDefault()
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "configuration written to %s\n", p)
				return nil
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(map[string]any{"path": config.Path(), "config": cfg})
			}
			fmt.Fprintf(out, "%s\n", g.th.Title("Configuration"))
			fmt.Fprintf(out, "  file:     %s\n", config.Path())
			fmt.Fprintf(out, "  excludes: %s\n", listOrNone(cfg.Excludes))
			fmt.Fprintf(out, "  protected: %s\n", listOrNone(cfg.Protected))
			fmt.Fprintf(out, "  scan depth: %d\n", cfg.ScanDepth)
			fmt.Fprintf(out, "  top rows:   %d\n", cfg.Top)
			fmt.Fprintf(out, "  trash by default: %t\n", cfg.TrashByDefault)
			fmt.Fprintf(out, "  audit log: %s\n", auditPath())
			return nil
		},
	}
	cmd.Flags().BoolVar(&initOnly, "init", false, "create the configuration file with defaults")
	return cmd
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

// confirm asks the user for confirmation. It returns false when the answer is
// not an explicit "yes". With --yes it always returns true. When stdin is not a
// terminal the answer is treated as "no".
func confirm(question string) bool {
	if g.yes {
		return true
	}
	if !style.IsTerminal(os.Stdin) && !isPipeAnswer() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s %s ", g.th.Yellow("?"), question)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes" || answer == "да" || answer == "д"
}

// maxConfirmItems is the number of rows shown before a confirmation prompt.
const maxConfirmItems = 25

// printItemsTable prints a table of paths with sizes, truncating long lists.
func printItemsTable(w io.Writer, header []string, rows [][]string, total int) {
	if total > maxConfirmItems {
		rows = rows[:maxConfirmItems]
	}
	t := &style.Table{Headers: header, Rows: rows, Align: []style.Align{style.Right, style.Left}, MaxWidth: []int{12, 120}}
	fmt.Fprint(w, t.Render(g.th))
	if total > maxConfirmItems {
		fmt.Fprintf(w, "%s\n", g.th.Dim(fmt.Sprintf("… and %d more", total-maxConfirmItems)))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
