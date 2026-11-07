// Package units converts and formats sizes, rates and durations for display
// and for parsing values coming from the command line.
package units

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// HumanBytes renders a byte count using decimal (SI) units, matching the way
// macOS Finder and Windows Explorer report sizes: 1 KB = 1000 B.
func HumanBytes(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	v := float64(n)
	un := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	i := 0
	for v >= 1000 && i < len(un)-1 {
		v /= 1000
		i++
	}
	var s string
	switch {
	case i == 0:
		s = fmt.Sprintf("%d B", n)
	case v < 10:
		s = fmt.Sprintf("%.2f %s", v, un[i])
	case v < 100:
		s = fmt.Sprintf("%.1f %s", v, un[i])
	default:
		s = fmt.Sprintf("%.0f %s", v, un[i])
	}
	if neg {
		return "-" + s
	}
	return s
}

// HumanRate renders a throughput value (bytes per second) with a "/s" suffix.
func HumanRate(bps float64) string {
	if bps < 0 || math.IsNaN(bps) || math.IsInf(bps, 0) {
		bps = 0
	}
	return HumanBytes(int64(bps)) + "/s"
}

// HumanCount renders an integer with thin grouping separators.
func HumanCount(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// HumanDuration renders a coarse, human friendly duration such as "3d 4h".
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int64(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int64(d / time.Minute)
	d -= time.Duration(mins) * time.Minute
	secs := int64(d / time.Second)
	millis := int64(d/time.Millisecond) % 1000

	type part struct {
		v      int64
		suffix string
	}
	var parts []part
	switch {
	case days > 0:
		parts = []part{{days, "d"}, {hours, "h"}}
	case hours > 0:
		parts = []part{{hours, "h"}, {mins, "m"}}
	case mins > 0:
		parts = []part{{mins, "m"}, {secs, "s"}}
	default:
		parts = []part{{secs, "s"}, {millis, "ms"}}
	}
	out := ""
	for _, p := range parts {
		if p.v == 0 {
			continue
		}
		if out != "" && len(strings.Fields(out)) >= 2 {
			break
		}
		if out != "" {
			out += " "
		}
		out += strconv.FormatInt(p.v, 10) + p.suffix
	}
	if out == "" {
		return "0s"
	}
	return out
}

// sizeUnits maps a (lower case) unit suffix to its byte multiplier.
var sizeUnits = map[string]float64{
	"":    1,
	"b":   1,
	"k":   1e3,
	"kb":  1e3,
	"kib": 1024,
	"m":   1e6,
	"mb":  1e6,
	"mib": 1024 * 1024,
	"g":   1e9,
	"gb":  1e9,
	"gib": 1024 * 1024 * 1024,
	"t":   1e12,
	"tb":  1e12,
	"tib": 1024 * 1024 * 1024 * 1024,
}

// ParseBytes parses human readable sizes such as "10MB", "1.5 GiB", "512" or
// "2g" into a byte count.
func ParseBytes(s string) (int64, error) {
	orig := s
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ',' || s[i] == '_') {
		i++
	}
	numPart := strings.NewReplacer(",", "", "_", "").Replace(s[:i])
	if numPart == "" {
		return 0, fmt.Errorf("invalid size %q", orig)
	}
	unit := s[i:]
	mult, ok := sizeUnits[unit]
	if !ok {
		return 0, fmt.Errorf("unknown size unit %q in %q", unit, orig)
	}
	v, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", orig)
	}
	if v < 0 {
		return 0, fmt.Errorf("negative size %q", orig)
	}
	return int64(math.Round(v * mult)), nil
}

// ParseAge parses durations that may use day or week suffixes ("30d", "2w",
// "12h", "90m"), which time.ParseDuration does not support.
func ParseAge(s string) (time.Duration, error) {
	orig := s
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var total time.Duration
	rest := s
	sawAny := false
	for rest != "" {
		i := 0
		for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9' || rest[i] == '.') {
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("invalid duration %q", orig)
		}
		v, err := strconv.ParseFloat(rest[:i], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", orig)
		}
		rest = rest[i:]
		j := 0
		for j < len(rest) && rest[j] >= 'a' && rest[j] <= 'z' {
			j++
		}
		unit := rest[:j]
		rest = rest[j:]
		var mult time.Duration
		switch unit {
		case "d", "day", "days":
			mult = 24 * time.Hour
		case "w", "week", "weeks":
			mult = 7 * 24 * time.Hour
		case "mo", "month", "months":
			mult = 30 * 24 * time.Hour
		case "y", "year", "years":
			mult = 365 * 24 * time.Hour
		case "ns", "us", "µs", "ms", "s", "m", "h":
			d, err := time.ParseDuration("1" + unit)
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q", orig)
			}
			mult = d
		case "":
			return 0, fmt.Errorf("missing unit in %q", orig)
		default:
			return 0, fmt.Errorf("unknown duration unit %q in %q", unit, orig)
		}
		total += time.Duration(math.Round(v * float64(mult)))
		sawAny = true
	}
	if !sawAny {
		return 0, fmt.Errorf("invalid duration %q", orig)
	}
	return total, nil
}

// Percent renders a 0..100 ratio as a padded percentage string.
func Percent(v float64, decimals int) string {
	return fmt.Sprintf("%.*f%%", decimals, v)
}
