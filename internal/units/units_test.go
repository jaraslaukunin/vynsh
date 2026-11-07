package units

import (
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1000, "1.00 KB"},
		{1500, "1.50 KB"},
		{15000, "15.0 KB"},
		{150000, "150 KB"},
		{1500000, "1.50 MB"},
		{5 * 1000 * 1000 * 1000, "5.00 GB"},
		{-2048, "-2.05 KB"},
	}
	for _, c := range cases {
		if got := HumanBytes(c.in); got != c.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"10MB", 10_000_000},
		{"10 mb", 10_000_000},
		{"1.5GiB", 1610612736},
		{"512", 512},
		{"2g", 2_000_000_000},
		{"0", 0},
	}
	for _, c := range cases {
		got, err := ParseBytes(c.in)
		if err != nil {
			t.Fatalf("ParseBytes(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	if _, err := ParseBytes("banana"); err == nil {
		t.Error("expected error for invalid size")
	}
	if _, err := ParseBytes("10xb"); err == nil {
		t.Error("expected error for unknown unit")
	}
}

func TestParseAge(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := ParseAge(c.in)
		if err != nil {
			t.Fatalf("ParseAge(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseAge(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "d", "10", "10x"} {
		if _, err := ParseAge(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{1500 * time.Millisecond, "1s 500ms"},
		{36 * time.Hour, "1d 12h"},
		{90 * time.Second, "1m 30s"},
	}
	for _, c := range cases {
		if got := HumanDuration(c.in); got != c.want {
			t.Errorf("HumanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
