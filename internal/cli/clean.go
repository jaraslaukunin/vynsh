package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/clean"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newCleanCmd() *cobra.Command {
	var (
		only         []string
		category     []string
		includeHigh  bool
		includeAdmin bool
		list         bool
		dryRun       bool
	)
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove caches, logs and other regenerable junk",
		Long: `Measure and remove well-known junk: build caches, package manager caches,
browser caches, logs, crash reports and temporary files.

vynsh first measures and prints what it found. In a terminal it then asks for
confirmation; in scripts (no terminal) it stays a dry run unless --yes is given,
and --dry-run always means "measure only". Rules marked "high" risk (Xcode
archives, device support, package repositories) are skipped unless you pass
--high or name them with --only.

The contents of cache directories are removed, the directories themselves are
kept so applications keep working.

Examples:
  vynsh clean                          # measure everything, change nothing
  vynsh clean --category dev           # only developer caches
  vynsh clean --only go-build-cache
  vynsh clean --high                  # include risky rules
  vynsh clean --admin                 # also system paths (sudo)
  vynsh clean --list                   # show the rule catalogue`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			rules, err := clean.Select(only, category, includeHigh, includeAdmin)
			if err != nil {
				return err
			}
			if list {
				if g.json {
					return mustJSON(rules)
				}
				printRuleCatalogue(out, rules)
				return nil
			}
			sp := style.NewSpinner(os.Stderr, g.th, "measuring cleanup targets")
			sp.Start()
			targets := clean.Discover(rules, clean.DiscoverOptions{
				Progress: func(done, total int, path string) {
					sp.Update(fmt.Sprintf("measuring %d/%d — %s", done, total, shortenPath(path)))
				},
			})
			sp.Stop()

			var present []clean.Target
			for _, t := range targets {
				if t.Exists && t.Size > 0 {
					present = append(present, t)
				}
			}
			if g.json {
				delete := g.yes && !dryRun
				if !delete {
					return mustJSON(map[string]any{
						"dry_run": true,
						"total":   clean.Total(present),
						"targets": present,
					})
				}
				rep := clean.Apply(present, false, auditCleanItem)
				return mustJSON(rep)
			}
			cats := make([]string, 0, len(clean.Categories()))
			cats = append(cats, clean.Categories()...)
			total := printCleanRules(out, present, cats)

			fmt.Fprintf(out, "%s %s\n", g.th.Title("Total reclaimable:"), g.th.Bold(units.HumanBytes(total)))
			if !includeAdmin && len(only) == 0 {
				if n := len(clean.AdminRules()); n > 0 {
					fmt.Fprintf(out, "%s\n", g.th.Dim(fmt.Sprintf(
						"%s need administrator rights and are skipped; add --admin (or run with sudo) to include them.",
						plural(int64(n), "rule", "rules"))))
				}
			}
			if len(present) == 0 {
				fmt.Fprintf(out, "%s\n", g.th.Dim("Nothing to clean — you are already tidy."))
				return nil
			}
			if dryRun {
				printCleanReport(out, clean.Apply(present, true, nil))
				return nil
			}
			if !mayDelete(out,
				fmt.Sprintf("Remove %s of caches across %s?", units.HumanBytes(total),
					plural(int64(len(present)), "target", "targets")),
				"Dry run: nothing was deleted. Re-run in a terminal to confirm, or pass --yes for unattended cleanup.") {
				return nil
			}
			before := freeSpace(fsutil.HomeDir())
			rep := clean.Apply(present, false, auditCleanItem)
			printCleanReport(out, rep)
			after := freeSpace(fsutil.HomeDir())
			printSpaceLine(out, before, after)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&only, "only", nil, "comma separated rule ids to apply")
	cmd.Flags().StringSliceVar(&category, "category", nil, "limit to categories: dev, browsers, apps, logs, system, temp")
	cmd.Flags().BoolVar(&includeHigh, "high", false, "include high-risk rules (archives, package repositories)")
	cmd.Flags().BoolVar(&includeAdmin, "admin", false, "include rules for system paths (needs root / administrator)")
	cmd.Flags().BoolVar(&list, "list", false, "list the available rules without measuring")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "measure only, never delete (overrides --yes)")
	return cmd
}

func printRuleCatalogue(w io.Writer, rules []clean.Rule) {
	rows := make([][]string, 0, len(rules))
	for _, r := range rules {
		rows = append(rows, []string{r.ID, r.Category, r.Risk.String(), r.Name, r.Note})
	}
	t := &style.Table{
		Headers:  []string{"ID", "CATEGORY", "RISK", "WHAT", "NOTE"},
		Rows:     rows,
		Align:    []style.Align{style.Left, style.Left, style.Left, style.Left, style.Left},
		MaxWidth: []int{24, 10, 8, 34, 60},
	}
	fmt.Fprint(w, t.Render(g.th))
	fmt.Fprintf(w, "\n%s\n", g.th.Dim(fmt.Sprintf("%d rules for %s", len(rules), osName())))
}

// freeSpace returns free bytes on the volume holding path.
func freeSpace(path string) uint64 {
	if path == "" {
		path = "."
	}
	u, err := disk.Usage(path)
	if err != nil || u == nil {
		return 0
	}
	return u.Free
}

// auditCleanItem records one cleaned target in the audit log.
func auditCleanItem(it clean.Item) {
	entry := audit.Entry{Action: "clean", Path: it.Path, Bytes: it.Bytes, Result: "ok"}
	if it.Err != "" {
		entry.Result = "error"
		entry.Error = it.Err
	}
	_ = audit.Log(entry)
}
