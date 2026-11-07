package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/style"
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
  vynsh trash add ./old.log
  vynsh trash restore --all   # put back what vynsh moved to the trash`,
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
	cmd.AddCommand(newTrashEmptyCmd(), newTrashAddCmd(), newTrashRestoreCmd())
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

func newTrashRestoreCmd() *cobra.Command {
	var (
		all    bool
		ids    []int64
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Put files back from the trash (items moved there by vynsh)",
		Long: `Restore items that vynsh moved to the trash.

Windows and macOS do not expose a portable "where did this come from" record, so
vynsh keeps its own manifest of everything it moved to the trash. This command
restores those items to their original paths; if the original path is taken, the
item is restored next to it with a numbered name.

Items that other applications deleted can be restored with Finder (⌘Z / "Put
Back") or Explorer.

Examples:
  vynsh trash restore              # list what can be restored
  vynsh trash restore --all        # restore everything (asks first)
  vynsh trash restore --id 1,3
  vynsh trash restore --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			records, err := trash.Restorable()
			if err != nil {
				return err
			}
			if g.json {
				if len(records) == 0 || (!all && len(ids) == 0) {
					return mustJSON(records)
				}
				selected := pickRecords(records, all, ids)
				rep, err := trash.Restore(selected, dryRun || !g.yes)
				if err != nil {
					return err
				}
				return mustJSON(rep)
			}
			if len(records) == 0 {
				fmt.Fprintf(out, "%s\n", g.th.Dim("Nothing to restore: vynsh has no record of items it moved to the trash."))
				fmt.Fprintf(out, "%s\n", g.th.Dim("Items deleted by other apps can be put back with Finder (“Put Back”) or Explorer."))
				return nil
			}
			rows := make([][]string, 0, len(records))
			for i, r := range records {
				kind := "file"
				if r.IsDir {
					kind = "dir "
				}
				rows = append(rows, []string{
					itoa(i + 1),
					r.Time.Format("2006-01-02 15:04"),
					units.HumanBytes(r.Bytes),
					kind,
					shortenHome(r.Original),
				})
			}
			t := &style.Table{
				Headers:  []string{"#", "WHEN", "SIZE", "KIND", "ORIGINAL PATH"},
				Rows:     rows,
				Align:    []style.Align{style.Right, style.Left, style.Right, style.Left, style.Left},
				MaxWidth: []int{4, 19, 10, 4, 80},
			}
			fmt.Fprint(out, t.Render(g.th))
			fmt.Fprintf(out, "\n%s\n", g.th.Dim(fmt.Sprintf("%s can be restored to their original location",
				plural(int64(len(records)), "item", "items"))))

			selected := pickRecords(records, all, ids)
			if len(selected) == 0 && !all && len(ids) == 0 {
				fmt.Fprintf(out, "%s\n", g.th.Dim("Use --all or --id 1,2 to restore."))
				return nil
			}
			if len(selected) == 0 {
				return fmt.Errorf("no matching items to restore")
			}
			if dryRun {
				rep, err := trash.Restore(selected, true)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "\n%s\n", g.th.Dim("This was a dry run: nothing was restored."))
				for _, r := range rep.Results {
					fmt.Fprintf(out, "  %s → %s\n", shortenHome(r.Record.Original), shortenHome(r.Restored))
				}
				return nil
			}
			if !mayDelete(out, fmt.Sprintf("Restore %s?", plural(int64(len(selected)), "item", "items")),
				"Nothing was restored. Re-run in a terminal to confirm, or pass --yes.") {
				return nil
			}
			rep, err := trash.Restore(selected, false)
			if err != nil {
				return err
			}
			for _, r := range rep.Results {
				switch {
				case r.Err != "":
					fmt.Fprintf(out, "  %s %s: %s\n", g.th.Red("✗"), shortenHome(r.Record.Original), g.th.Dim(r.Err))
				case r.Restored != r.Record.Original:
					fmt.Fprintf(out, "  %s %s %s\n", g.th.Green("✓"), shortenHome(r.Restored),
						g.th.Dim("(original path was taken)"))
				default:
					fmt.Fprintf(out, "  %s %s\n", g.th.Green("✓"), shortenHome(r.Restored))
				}
				_ = audit.Log(audit.Entry{Action: "trash-restore", Path: r.Record.Original, Bytes: r.Record.Bytes, Result: resultOf(r.Err)})
			}
			fmt.Fprintf(out, "\n%s restored %s\n", g.th.Green("✓"), g.th.Bold(units.HumanBytes(rep.Bytes)))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "restore every item vynsh can restore")
	cmd.Flags().Int64SliceVar(&ids, "id", nil, "restore specific items by number (1,3)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be restored")
	return cmd
}

// pickRecords selects manifest entries by --all or by 1-based index.
func pickRecords(records []trash.Record, all bool, ids []int64) []trash.Record {
	if all {
		return records
	}
	var out []trash.Record
	for _, id := range ids {
		if id >= 1 && int(id) <= len(records) {
			out = append(out, records[id-1])
		}
	}
	return out
}

func resultOf(errText string) string {
	if errText == "" {
		return "ok"
	}
	return "error"
}

// shortenHome renders an absolute path relative to the home directory.
func shortenHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, rerr := filepath.Rel(home, p); rerr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if rel == "." {
			return "~"
		}
		return "~" + string(filepath.Separator) + rel
	}
	return p
}
