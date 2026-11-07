package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newAuditCmd() *cobra.Command {
	var (
		tail int
		path bool
	)
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show the log of destructive actions",
		Long: `vynsh records every cleanup, deletion and trash operation it performs in an
append-only JSON Lines log. Use it to find out what was removed, when, and how
much space it freed.

Examples:
  vynsh audit
  vynsh audit --tail 200
  vynsh audit --path`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if path {
				fmt.Fprintln(out, auditPath())
				return nil
			}
			entries, err := audit.Tail(tail)
			if err != nil {
				return err
			}
			if g.json {
				return mustJSON(entries)
			}
			if len(entries) == 0 {
				fmt.Fprintf(out, "%s\n", g.th.Dim("No actions recorded yet ("+auditPath()+")."))
				return nil
			}
			fmt.Fprintf(out, "%s\n", g.th.Dim(auditPath()))
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				result := g.th.Green("ok")
				if e.Result != "ok" {
					result = g.th.Red(e.Result)
				}
				rows = append(rows, []string{
					e.Time.Format("2006-01-02 15:04:05"),
					e.Action,
					result,
					units.HumanBytes(e.Bytes),
					e.Path,
				})
			}
			t := &style.Table{
				Headers:  []string{"WHEN", "ACTION", "RESULT", "SIZE", "PATH"},
				Rows:     rows,
				Align:    []style.Align{style.Left, style.Left, style.Left, style.Right, style.Left},
				MaxWidth: []int{19, 12, 8, 10, 80},
			}
			fmt.Fprint(out, t.Render(g.th))
			return nil
		},
	}
	cmd.Flags().IntVarP(&tail, "tail", "n", 25, "how many recent entries to show (0 = all)")
	cmd.Flags().BoolVar(&path, "path", false, "print the log file path only")
	return cmd
}
