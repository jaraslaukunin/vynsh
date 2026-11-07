// Package style provides terminal styling, tables and progress helpers that
// degrade gracefully when output is redirected to a file or a dumb terminal.
package style

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	white   = "\x1b[97m"
)

// Theme applies ANSI colours when enabled and is a no-op otherwise.
type Theme struct {
	Enabled bool
}

// Default returns a theme configured from the environment: colours are on only
// when stdout is a terminal, TERM is not dumb and NO_COLOR is unset.
func Default() *Theme { return For(os.Stdout, os.Getenv("NO_COLOR") != "") }

// For returns a theme for the given writer.
func For(w io.Writer, noColor bool) *Theme {
	if noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return &Theme{}
	}
	f, ok := w.(*os.File)
	if !ok {
		return &Theme{}
	}
	return &Theme{Enabled: isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())}
}

func (t *Theme) wrap(code, s string) string {
	if t == nil || !t.Enabled || s == "" {
		return s
	}
	return code + s + reset
}

func (t *Theme) Bold(s string) string    { return t.wrap(bold, s) }
func (t *Theme) Dim(s string) string     { return t.wrap(dim, s) }
func (t *Theme) Red(s string) string     { return t.wrap(red, s) }
func (t *Theme) Green(s string) string   { return t.wrap(green, s) }
func (t *Theme) Yellow(s string) string  { return t.wrap(yellow, s) }
func (t *Theme) Blue(s string) string    { return t.wrap(blue, s) }
func (t *Theme) Magenta(s string) string { return t.wrap(magenta, s) }
func (t *Theme) Cyan(s string) string    { return t.wrap(cyan, s) }
func (t *Theme) White(s string) string   { return t.wrap(white, s) }

// Title renders a section heading.
func (t *Theme) Title(s string) string { return t.wrap(bold+cyan, s) }

// Header renders a table header row.
func (t *Theme) Header(s string) string { return t.wrap(bold+white, s) }

// Width returns the printable width of s, ignoring ANSI escapes.
func Width(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			w += runewidth.RuneWidth(r)
		}
	}
	return w
}

// Pad right-pads s with spaces up to width w (visible width aware).
func Pad(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// PadLeft left-pads s with spaces up to width w.
func PadLeft(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// Truncate shortens s to at most w visible columns, adding an ellipsis.
func Truncate(s string, w int) string {
	if Width(s) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if cur+rw > w-1 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "…"
}

// Align describes column alignment in a Table.
type Align int

const (
	Left Align = iota
	Right
)

// Table renders a simple aligned table without external dependencies.
type Table struct {
	Headers []string
	Rows    [][]string
	Align   []Align
	Indent  string
	Gap     int
	// MaxWidth per column; zero means unlimited.
	MaxWidth []int
}

// Render returns the formatted table as a string.
func (t *Table) Render(th *Theme) string {
	if len(t.Rows) == 0 && len(t.Headers) == 0 {
		return ""
	}
	gap := t.Gap
	if gap <= 0 {
		gap = 2
	}
	cols := len(t.Headers)
	for _, r := range t.Rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for i, h := range t.Headers {
		widths[i] = Width(h)
	}
	for _, r := range t.Rows {
		for i, c := range r {
			if w := Width(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		if i < len(t.MaxWidth) && t.MaxWidth[i] > 0 && widths[i] > t.MaxWidth[i] {
			widths[i] = t.MaxWidth[i]
		}
	}
	alignAt := func(i int) Align {
		if i < len(t.Align) {
			return t.Align[i]
		}
		return Left
	}
	var b strings.Builder
	line := func(cells []string, hdr bool) {
		b.WriteString(t.Indent)
		for i := 0; i < cols; i++ {
			var cell string
			if i < len(cells) {
				cell = cells[i]
			}
			if i < len(t.MaxWidth) && t.MaxWidth[i] > 0 {
				cell = Truncate(cell, t.MaxWidth[i])
			}
			if hdr {
				cell = th.Header(cell)
			}
			if alignAt(i) == Right {
				b.WriteString(PadLeft(cell, widths[i]))
			} else {
				b.WriteString(Pad(cell, widths[i]))
			}
			if i != cols-1 {
				b.WriteString(strings.Repeat(" ", gap))
			}
		}
		b.WriteString("\n")
	}
	if len(t.Headers) > 0 {
		line(t.Headers, true)
		sep := make([]string, cols)
		for i := range sep {
			sep[i] = th.Dim(strings.Repeat("─", widths[i]))
		}
		line(sep, false)
	}
	for _, r := range t.Rows {
		line(r, false)
	}
	return b.String()
}

// Bar renders a horizontal proportion bar of the given width.
func Bar(fraction float64, width int, th *Theme) string {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(fraction*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	s := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	switch {
	case fraction >= 0.9:
		return th.Red(s)
	case fraction >= 0.75:
		return th.Yellow(s)
	default:
		return th.Green(s)
	}
}

// Spinner prints a single-line progress indicator on a terminal.
type Spinner struct {
	w       io.Writer
	enabled bool
	mu      sync.Mutex
	label   string
	done    chan struct{}
	started bool
	th      *Theme
}

// NewSpinner returns a spinner writing to w. An empty label uses the default.
func NewSpinner(w io.Writer, th *Theme, label string) *Spinner {
	f, ok := w.(*os.File)
	enabled := ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())) && os.Getenv("TERM") != "dumb"
	return &Spinner{w: w, enabled: enabled, th: th, label: label}
}

// Start begins animating. It is safe to call on a disabled spinner.
func (s *Spinner) Start() {
	if !s.enabled || s.started {
		return
	}
	s.started = true
	s.done = make(chan struct{})
	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		tk := time.NewTicker(90 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-tk.C:
				s.mu.Lock()
				fmt.Fprintf(s.w, "\r\x1b[2K%s %s", s.th.Cyan(frames[i%len(frames)]), s.label)
				s.mu.Unlock()
				i++
			}
		}
	}()
}

// Stop halts the animation and clears the line.
func (s *Spinner) Stop() {
	if !s.enabled || !s.started {
		return
	}
	close(s.done)
	s.started = false
	s.mu.Lock()
	fmt.Fprint(s.w, "\r\x1b[2K")
	s.mu.Unlock()
}

// Update changes the spinner label.
func (s *Spinner) Update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// IsTerminal reports whether w is an interactive terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
