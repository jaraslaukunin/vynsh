package clean

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelectByCategory(t *testing.T) {
	rules, err := Select(nil, []string{"dev"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("expected dev rules")
	}
	for _, r := range rules {
		if r.Category != "dev" {
			t.Errorf("rule %s has category %s, want dev", r.ID, r.Category)
		}
	}
	if _, err := Select(nil, []string{"nope"}, true); err == nil {
		t.Error("expected error for unknown category")
	}
}

func TestSelectByID(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("no rules for this platform")
	}
	id := all[0].ID
	rules, err := Select([]string{id}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != id {
		t.Fatalf("rules = %+v, want just %s", rules, id)
	}
}

func TestSelectHighRiskGating(t *testing.T) {
	withHigh, err := Select(nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	withoutHigh, err := Select(nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	high := 0
	for _, r := range withHigh {
		if r.Risk == RiskHigh {
			high++
		}
	}
	if high == 0 {
		t.Skip("no high risk rules on this platform")
	}
	if len(withoutHigh) >= len(withHigh) {
		t.Errorf("high risk rules were not filtered: %d vs %d", len(withoutHigh), len(withHigh))
	}
	for _, r := range withoutHigh {
		if r.Risk == RiskHigh {
			t.Errorf("high risk rule %s leaked into the default selection", r.ID)
		}
	}
}

func TestDiscoverAndApply(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	// Build a fake cache tree matching the current platform's rules.
	cacheDir := filepath.Join(home, "Library", "Caches")
	if runtime.GOOS == "windows" {
		cacheDir = filepath.Join(home, "AppData", "Local")
	}
	nested := filepath.Join(cacheDir, "vynsh-test-child")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "blob.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	rules := []Rule{{
		ID: "test-rule", Name: "Test rule", Category: "dev", Risk: RiskLow,
		Paths: []string{filepath.Join(home, "Library", "Caches", "*")},
	}}
	targets := Discover(rules, DiscoverOptions{})
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want 1", targets)
	}
	if targets[0].Size != 4096 {
		t.Errorf("size = %d, want 4096", targets[0].Size)
	}

	// Dry run must not touch the disk.
	rep := Apply(targets, true, nil)
	if rep.Bytes != 4096 {
		t.Errorf("dry run bytes = %d, want 4096", rep.Bytes)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("dry run removed data: %v", err)
	}

	// Real run empties the directory but keeps it.
	rep = Apply(targets, false, nil)
	if len(rep.Failed) != 0 {
		t.Fatalf("failed = %+v", rep.Failed)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("directory should still exist: %v", err)
	}
	ents, err := os.ReadDir(nested)
	if err != nil || len(ents) != 0 {
		t.Fatalf("directory not emptied: %v, %d entries", err, len(ents))
	}
}

func TestDiscoverDeduplicatesNestedTargets(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	inner := filepath.Join(home, "caches", "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "x.bin"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := []Rule{
		{ID: "specific", Name: "specific", Category: "dev", Paths: []string{inner}},
		{ID: "broad", Name: "broad", Category: "dev", Paths: []string{filepath.Join(home, "caches", "*")}},
	}
	targets := Discover(rules, DiscoverOptions{})
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want only the specific one", targets)
	}
	if targets[0].Rule.ID != "specific" {
		t.Errorf("kept rule = %s, want specific", targets[0].Rule.ID)
	}
	if Total(targets) != 1000 {
		t.Errorf("total = %d, want 1000 (no double counting)", Total(targets))
	}
}

func TestApplyRefusesProtectedPaths(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	// The home directory itself must never be a cleanup target.
	targets := []Target{{Rule: Rule{ID: "x"}, Path: home, Exists: true, IsDir: true, Size: 1}}
	rep := Apply(targets, false, nil)
	if len(rep.Failed) != 1 {
		t.Fatalf("report = %+v, want one failure", rep)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("home was deleted: %v", err)
	}
}

func TestCategoriesStable(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("no categories")
	}
	for i := 1; i < len(cats); i++ {
		if cats[i-1] > cats[i] {
			t.Fatalf("categories are not sorted: %v", cats)
		}
	}
}
