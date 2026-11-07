package cli

import (
	"fmt"
	"image/png"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/logo"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

func newLogoCommand() *cobra.Command {
	var (
		out         string
		size        int
		padding     int
		bg          string
		transparent bool
		ascii       bool
		variant     string
		format      string
		hero        string
		gap         int
		tight       bool
	)
	cmd := &cobra.Command{
		Use:   "logo",
		Short: "Export the vynsh beetle as a square PNG (branding)",
		Long: `Render the vynsh mascot as pixel art.

The beetle is drawn from the same sprite that the terminal dashboard uses, so
the logo and the app always match. Pixels are scaled with integer factors, which
keeps the art perfectly crisp at any size.

Examples:
  vynsh logo                          # vynsh-logo-1024.png, transparent
  vynsh logo --size 512 --bg "#0d0b1a"
  vynsh logo --variant hop --out hop.png
  vynsh logo --ascii                  # print it in the terminal`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sprite, ok := logo.All()[strings.ToLower(variant)]
			if hero != "" {
				// Compose the mascot with the pixel wordmark into one lockup.
				sprite = logo.Hero(logo.Base, hero, gap)
				ok = true
			}
			if !ok {
				names := make([]string, 0, 3)
				for name := range logo.All() {
					names = append(names, name)
				}
				return fmt.Errorf("unknown variant %q (try: %s)", variant, strings.Join(names, ", "))
			}
			if err := logo.Check(); err != nil {
				return err
			}
			if ascii {
				fmt.Fprint(cmd.OutOrStdout(), logo.ASCII(sprite, '█', ' '))
				return nil
			}
			if size <= 0 {
				size = 1024
			}
			if padding < 0 {
				padding = 0
			}
			if padding == 0 && !cmd.Flags().Changed("padding") {
				padding = size / 10
			}
			opts := logo.Options{
				Size:        size,
				Padding:     padding,
				Transparent: transparent,
				Tight:       tight || hero != "",
			}
			if !transparent {
				c, err := logo.ParseHex(bg)
				if err != nil {
					return err
				}
				opts.Background = logo.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
			}
			if format == "" {
				format = "png"
				if strings.HasSuffix(strings.ToLower(out), ".svg") {
					format = "svg"
				}
			}
			if out == "" {
				out = fmt.Sprintf("vynsh-logo-%d.%s", size, format)
			}
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			defer f.Close()
			var dims string
			switch format {
			case "svg":
				// size is the display width in pixels for vector output
				var bg *logo.RGBA
				if !transparent {
					bg = &opts.Background
				}
				if _, err := f.WriteString(logo.SVG(sprite, bg, size)); err != nil {
					return err
				}
				dims = fmt.Sprintf("%dx%d", size, size*sprite.Height()/sprite.Width())
			case "png":
				img := logo.PNG(sprite, opts)
				if err := png.Encode(f, img); err != nil {
					return err
				}
				dims = fmt.Sprintf("%dx%d", img.Bounds().Dx(), img.Bounds().Dy())
			default:
				return fmt.Errorf("unknown format %q (png or svg)", format)
			}
			info, _ := f.Stat()
			fmt.Fprintf(cmd.OutOrStdout(), "%s wrote %s (%s, %s)\n", g.th.Green("✓"), out,
				dims, humanFileSize(info))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file (default vynsh-logo-<size>.png)")
	cmd.Flags().IntVar(&size, "size", 1024, "square image size in pixels")
	cmd.Flags().IntVar(&padding, "padding", 0, "margin around the beetle (default 10% of size)")
	cmd.Flags().StringVar(&bg, "bg", "#0d0b1a", "background colour when --bg is used instead of transparency")
	cmd.Flags().BoolVar(&transparent, "transparent", true, "transparent background")
	cmd.Flags().BoolVar(&ascii, "ascii", false, "print the sprite in the terminal instead of writing a PNG")
	cmd.Flags().StringVar(&variant, "variant", "base", "which pose to export: base, hop, blink, wink, antennae, type1")
	cmd.Flags().StringVar(&format, "format", "", "png or svg (inferred from the file extension)")
	cmd.Flags().StringVar(&hero, "hero", "", `compose the mascot with a wordmark, e.g. --hero vynsh`)
	cmd.Flags().IntVar(&gap, "gap", 3, "space between the mascot and the wordmark, in pixels")
	cmd.Flags().BoolVar(&tight, "tight", false, "crop the canvas to the art (no square padding)")
	return cmd
}

func humanFileSize(info os.FileInfo) string {
	if info == nil {
		return "?"
	}
	return units.HumanBytes(info.Size())
}
