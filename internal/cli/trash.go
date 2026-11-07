package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/trash"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newTrashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trash",
		Short: "Inspect, fill or empty the system trash",
		Long: `Work with the operating system trash: show what is in it, empty it, or move
files into it instead of deleting them for good.

Examples:
  vynsh trash                 # how much is in the trash
  vynsh trash empty           # show the size, then ask for confirmation
  vynsh trash empty --yes     # empty without asking (scripts)
  vynsh trash add ./old.log`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !trash.Supported() {
				return fmt.Errorf("trash is not supported on %s", osName())
			}
			rep, err := trash.Inspect()
			if err != nil {
				return err
			}
			if g.json {
				return mustJSON(rep)
			}
			printTrash(cmd.OutOrStdout(), rep)
			if rep.Items > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", g.th.Dim("empty it with: vynsh trash empty"))
			}
			return nil
		},
	}
	cmd.AddCommand(newTrashEmptyCmd(), newTrashAddCmd())
	return cmd
}

func newTrashEmptyCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "empty",
		Short: "Empty the trash",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if !trash.Supported() {
				return fmt.Errorf("trash is not supported on %s", osName())
			}
			if dryRun || !g.yes || g.json {
				rep, err := trash.Inspect()
				if err != nil {
					return err
				}
				if g.json {
					return mustJSON(rep)
				}
				printTrash(out, rep)
				if rep.Items == 0 {
					fmt.Fprintf(out, "%s\n", g.th.Dim("Trash is already empty."))
					return nil
				}
				if !dryRun {
					if !mayDelete(out, "Empty the trash permanently?",
						"Dry run: the trash was left untouched. Re-run in a terminal to confirm, or pass --yes.") {
						return nil
					}
				} else {
					fmt.Fprintf(out, "\n%s\n", g.th.Dim("This was a dry run: the trash was left untouched."))
					return nil
				}
			}
			rep, err := trash.Empty(false)
			if err != nil {
				if isPermissionErr(err.Error()) {
					return fmt.Errorf("%w\n%s", err, g.th.Dim("macOS protects the trash: grant your terminal “Full Disk Access” to empty it."))
				}
				return err
			}
			if g.json {
				return mustJSON(rep)
			}
			for _, l := range rep.Locations {
				_ = audit.Log(audit.Entry{Action: "trash-empty", Path: l.Path, Bytes: l.Bytes, Result: "ok"})
			}
			fmt.Fprintf(out, "%s %s removed from the trash (%s)\n",
				g.th.Green("✓"), g.th.Bold(units.HumanBytes(rep.Bytes)),
				plural(rep.Items, "item", "items"))
			fmt.Fprintln(out, g.th.Dim("Note: the operating system may keep the space until it needs it again."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "measure only, never delete")
	return cmd
}

func newTrashAddCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add <path ...>",
		Short: "Move files to the trash instead of deleting them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if !trash.Supported() {
				return fmt.Errorf("trash is not supported on %s", osName())
			}
			if dryRun || !g.yes {
				rep, err := trash.Move(args, true)
				if err != nil {
					return err
				}
				if g.json {
					return mustJSON(rep)
				}
				for _, m := range rep.Moved {
					state := "would move"
					if m.Err != "" {
						state = g.th.Red("error: " + m.Err)
					}
					fmt.Fprintf(out, "  %s %s → %s\n", units.HumanBytes(m.Bytes), m.Path, g.th.Dim(state))
				}
				if dryRun {
					fmt.Fprintf(out, "\n%s\n", g.th.Dim("This was a dry run: nothing was moved."))
					return nil
				}
				if !mayDelete(out, fmt.Sprintf("Move %s to the trash?", plural(int64(len(args)), "item", "items")),
					"Dry run: nothing was moved. Re-run in a terminal to confirm, or pass --yes.") {
					return nil
				}
			}
			rep, err := trash.Move(args, false)
			if err != nil {
				return err
			}
			if g.json {
				return mustJSON(rep)
			}
			failed := 0
			for _, m := range rep.Moved {
				if m.Err != "" {
					failed++
					fmt.Fprintf(out, "  %s %s\n", g.th.Red("✗"), m.Err+": "+m.Path)
					_ = audit.Log(audit.Entry{Action: "trash-add", Path: m.Path, Bytes: m.Bytes, Result: "error", Error: m.Err})
					continue
				}
				_ = audit.Log(audit.Entry{Action: "trash-add", Path: m.Path, Bytes: m.Bytes, Result: "ok"})
			}
			fmt.Fprintf(out, "%s moved %s to the trash (%s)\n",
				g.th.Green("✓"), g.th.Bold(units.HumanBytes(rep.Bytes)),
				plural(int64(len(rep.Moved)-failed), "item", "items"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would move, do not move anything")
	return cmd
}
