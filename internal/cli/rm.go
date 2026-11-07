package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/del"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newRmCmd() *cobra.Command {
	var (
		olderThan string
		newerThan string
		minSize   string
		maxSize   string
		names     []string
		exts      []string
		exclude   []string
		hidden    bool
		emptyDirs bool
		depth     int
		toTrash   bool
		limit     int64
		dryRun    bool
		noConfirm bool
	)
	cmd := &cobra.Command{
		Use:   "rm <path-or-glob ...>",
		Short: "Selectively delete files by age, size, name or type",
		Long: `Delete exactly the files you describe — never a whole tree by accident.

Filters are combined with AND: a file is selected only when every filter
matches. Directories are never deleted recursively; --empty-dirs removes
directories that are empty after the files have been removed.

Nothing is deleted unless you pass --yes (and even then the plan is printed and
confirmed interactively). With --trash the files are moved to the recycle bin
instead of being unlinked, so a mistake stays recoverable.

Examples:
  vynsh rm ~/Downloads --older-than 90d
  vynsh rm ~/Library/Caches --ext log --name "*.log" --min-size 10MB
  vynsh rm ./build --empty-dirs --trash
  vynsh rm /tmp --newer-than 1h --limit 1000
  vynsh rm ./data --name "draft-*.csv" --older-than 30d --yes`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			f := del.Filter{Files: true, Dirs: emptyDirs, IncludeHidden: hidden, MaxDepth: depth}
			if olderThan != "" {
				d, err := units.ParseAge(olderThan)
				if err != nil {
					return fmt.Errorf("--older-than: %w", err)
				}
				f.OlderThan = d
			}
			if newerThan != "" {
				d, err := units.ParseAge(newerThan)
				if err != nil {
					return fmt.Errorf("--newer-than: %w", err)
				}
				f.NewerThan = d
			}
			if minSize != "" {
				v, err := units.ParseBytes(minSize)
				if err != nil {
					return fmt.Errorf("--min-size: %w", err)
				}
				f.MinSize = v
			}
			if maxSize != "" {
				v, err := units.ParseBytes(maxSize)
				if err != nil {
					return fmt.Errorf("--max-size: %w", err)
				}
				f.MaxSize = v
			}
			f.Names = names
			f.Exts = exts
			f.ExcludeNames = exclude
			if f.OlderThan == 0 && f.NewerThan == 0 && f.MinSize == 0 && f.MaxSize == 0 &&
				len(names) == 0 && len(exts) == 0 && !dryRun && !noConfirm {
				fmt.Fprintf(out, "%s no filters given: every file under the given paths would be selected\n",
					g.th.Yellow("note:"))
			}

			sp := style.NewSpinner(os.Stderr, g.th, "building deletion plan")
			sp.Start()
			plan, err := del.Build(args, f, func(scanned int64, path string) {
				sp.Update(fmt.Sprintf("scanning — %s files, latest: %s", units.HumanCount(scanned), shortenPath(path)))
			})
			sp.Stop()
			if err != nil {
				return err
			}
			if g.json {
				if !g.yes || dryRun {
					return mustJSON(plan)
				}
				res := del.Apply(plan, del.ApplyOptions{
					ToTrash:   toTrash || g.cfg.TrashByDefault,
					Limit:     limit,
					PruneDirs: emptyDirs,
					Log:       auditItem,
				})
				return mustJSON(res)
			}

			if len(plan.Items) == 0 {
				fmt.Fprintf(out, "%s nothing matched — no files selected\n", g.th.Dim("plan:"))
				if len(plan.Errors) > 0 {
					for _, e := range plan.Errors {
						fmt.Fprintf(out, "  %s\n", g.th.Dim(e))
					}
				}
				return nil
			}
			printPlan(out, plan)
			if dryRun || !g.yes {
				fmt.Fprintf(out, "%s\n", g.th.Dim("This was a dry run. Re-run with --yes to delete, or add --trash to be able to undo."))
				return nil
			}
			useTrash := toTrash || g.cfg.TrashByDefault
			target := "delete permanently"
			if useTrash {
				target = "move to the trash"
			}
			if !confirm(fmt.Sprintf("%s %d items (%s)?", strings.ToUpper(target[:1])+target[1:], len(plan.Items), units.HumanBytes(plan.Bytes))) {
				fmt.Fprintln(out, g.th.Dim("aborted — nothing was deleted"))
				return nil
			}
			root := ""
			if len(plan.Roots) > 0 {
				root = plan.Roots[0]
			}
			before := freeSpace(root)
			res := del.Apply(plan, del.ApplyOptions{
				ToTrash:   useTrash,
				Limit:     limit,
				PruneDirs: emptyDirs,
				Log:       auditItem,
			})
			verb := "deleted"
			if useTrash {
				verb = "moved to trash"
			}
			fmt.Fprintf(out, "%s %s %s (%s", g.th.Green("✓"), verb, units.HumanBytes(res.Bytes), plural(res.Files, "file", "files"))
			if res.Dirs > 0 {
				fmt.Fprintf(out, " and %s", plural(res.Dirs, "empty dir", "empty dirs"))
			}
			fmt.Fprintln(out, ")")
			if res.Limited {
				fmt.Fprintf(out, "%s stopped at --limit %d\n", g.th.Yellow("note:"), limit)
			}
			if len(res.Failed) > 0 {
				fmt.Fprintf(out, "%s %d items could not be removed\n", g.th.Yellow("warning:"), len(res.Failed))
				for i, f := range res.Errors {
					if i >= 10 {
						fmt.Fprintf(out, "  … and %d more\n", len(res.Errors)-10)
						break
					}
					fmt.Fprintf(out, "  %s\n", g.th.Dim(f))
				}
			}
			if !useTrash {
				printSpaceLine(out, before, freeSpace(root))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "only files modified longer ago than this (30d, 6h, 2w)")
	cmd.Flags().StringVar(&newerThan, "newer-than", "", "only files modified more recently than this")
	cmd.Flags().StringVar(&minSize, "min-size", "", "only files at least this large (10MB)")
	cmd.Flags().StringVar(&maxSize, "max-size", "", "only files at most this large")
	cmd.Flags().StringSliceVar(&names, "name", nil, "glob pattern(s) for the file name, e.g. \"*.log\"")
	cmd.Flags().StringSliceVar(&exts, "ext", nil, "only these extensions (log,tmp,zip)")
	cmd.Flags().StringSliceVar(&exclude, "exclude-name", nil, "name globs to always keep")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include hidden files")
	cmd.Flags().BoolVar(&emptyDirs, "empty-dirs", false, "also remove directories that end up empty")
	cmd.Flags().IntVar(&depth, "depth", 0, "recurse at most this many levels (0 = unlimited)")
	cmd.Flags().BoolVar(&toTrash, "trash", false, "move to the recycle bin instead of deleting")
	cmd.Flags().Int64Var(&limit, "limit", 0, "stop after this many items")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan, delete nothing (overrides --yes)")
	cmd.Flags().BoolVar(&noConfirm, "no-confirm-hint", false, "hide the note about missing filters")
	return cmd
}

// auditItem records one deleted (or failed) item in the audit log.
func auditItem(it del.Item, err error) {
	e := audit.Entry{Action: "rm", Path: it.Path, Bytes: it.Size, Result: "ok"}
	if err != nil {
		e.Result = "error"
		e.Error = err.Error()
	}
	_ = audit.Log(e)
}
