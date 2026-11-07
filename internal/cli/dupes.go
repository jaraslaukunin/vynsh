package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/dupes"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newDupesCmd() *cobra.Command {
	var (
		minSize string
		maxSize string
		hidden  bool
	)
	cmd := &cobra.Command{
		Use:   "dupes [path ...]",
		Short: "Find duplicate files by content",
		Long: `Find identical files and how much space keeping one copy would free.

Files are compared in three passes — size, then the first 64 KiB, then a full
SHA-256 — so identical content is guaranteed and large trees stay fast.
vynsh only reports; it never deletes duplicates. Pipe the list into
"vynsh rm --name ..." or remove them yourself.

Examples:
  vynsh dupes ~/Pictures
  vynsh dupes . --min-size 50MB
  vynsh dupes ~ --json > dupes.json`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				args = []string{"."}
			}
			minBytes := int64(1)
			if minSize != "" {
				v, err := units.ParseBytes(minSize)
				if err != nil {
					return err
				}
				minBytes = v
			} else {
				minBytes = 1 << 20 // 1 MiB default: smaller files are rarely worth it
			}
			var maxBytes int64
			if maxSize != "" {
				v, err := units.ParseBytes(maxSize)
				if err != nil {
					return err
				}
				maxBytes = v
			}
			sp := style.NewSpinner(os.Stderr, g.th, "scanning for duplicates")
			sp.Start()
			last := time.Now()
			res, err := dupes.Find(args, dupes.Options{
				MinSize:       minBytes,
				MaxSize:       maxBytes,
				IncludeHidden: hidden,
				Exclude:       g.cfg.Excludes,
				Progress: func(phase string, done, total int64) {
					if time.Since(last) < 100*time.Millisecond {
						return
					}
					last = time.Now()
					if total > 0 {
						sp.Update(fmt.Sprintf("%s — %d/%d", phase, done, total))
					} else {
						sp.Update(fmt.Sprintf("%s — %d files", phase, done))
					}
				},
			})
			sp.Stop()
			if err != nil {
				return err
			}
			if g.json {
				return mustJSON(res)
			}
			printDupes(out, res)
			if len(res.Groups) == 0 {
				fmt.Fprintf(out, "%s\n", g.th.Dim("No duplicates found."))
			}
			if len(res.Errors) > 0 {
				fmt.Fprintf(out, "%s %d paths could not be read\n", g.th.Dim("note:"), len(res.Errors))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&minSize, "min-size", "1MB", "ignore files smaller than this")
	cmd.Flags().StringVar(&maxSize, "max-size", "", "ignore files larger than this")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include hidden files and directories")
	return cmd
}
