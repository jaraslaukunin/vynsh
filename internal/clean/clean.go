// Package clean implements the cache and junk cleanup engine: a rules registry
// per platform, size discovery and safe removal of rule targets.
package clean

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Risk describes how dangerous it is to remove a rule's data.
type Risk int

const (
	// RiskLow: data is regenerated automatically or is of no value.
	RiskLow Risk = iota
	// RiskMedium: data is re-downloaded or rebuilt, costing time or bandwidth.
	RiskMedium
	// RiskHigh: user data may be lost (archives, packages, device support).
	RiskHigh
)

func (r Risk) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	}
	return "unknown"
}

// Rule is one cleanup target definition.
type Rule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Risk     Risk     `json:"-"`
	RiskName string   `json:"risk"`
	Note     string   `json:"note,omitempty"`
	Paths    []string `json:"paths"`
	Admin    bool     `json:"needs_admin,omitempty"`
}

// Normalized returns the rule with RiskName populated.
func (r Rule) Normalized() Rule {
	r.RiskName = r.Risk.String()
	return r
}

// Target is a concrete path resolved from a rule.
type Target struct {
	Rule   Rule   `json:"rule"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Files  int64  `json:"files"`
	IsDir  bool   `json:"is_dir"`
	Exists bool   `json:"exists"`
}

// DiscoverOptions filters which rules are expanded.
type DiscoverOptions struct {
	IDs         []string
	Categories  []string
	IncludeHigh bool
	Progress    func(done, total int, path string)
}

// All returns every rule that applies to the current platform.
func All() []Rule {
	rs := platformRules()
	out := make([]Rule, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Normalized())
	}
	return out
}

// Select filters rules by id and category. High risk rules are only included
// when includeHigh is set (or the rule is named explicitly), and rules that need
// administrator rights only when includeAdmin is set.
func Select(ids, categories []string, includeHigh, includeAdmin bool) ([]Rule, error) {
	all := All()
	idSet := map[string]bool{}
	for _, id := range ids {
		for _, part := range strings.Split(id, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				idSet[part] = true
			}
		}
	}
	catSet := map[string]bool{}
	for _, c := range categories {
		for _, part := range strings.Split(c, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				catSet[strings.ToLower(part)] = true
			}
		}
	}
	var out []Rule
	for _, r := range all {
		if len(idSet) > 0 {
			if !idSet[r.ID] {
				continue
			}
			out = append(out, r) // explicitly named: take it as is
			continue
		}
		if len(catSet) > 0 && !catSet[strings.ToLower(r.Category)] {
			continue
		}
		if r.Risk == RiskHigh && !includeHigh {
			continue
		}
		if r.Admin && !includeAdmin {
			continue
		}
		out = append(out, r)
	}
	if len(idSet) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("no cleanup rule matches %v", ids)
	}
	if len(catSet) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("no cleanup rule in categories %v", categories)
	}
	return out, nil
}

// Categories returns the distinct categories in display order.
func Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range All() {
		if !seen[r.Category] {
			seen[r.Category] = true
			out = append(out, r.Category)
		}
	}
	sort.Strings(out)
	return out
}

// Discover resolves rule paths (expanding ~, environment variables and globs)
// and measures their sizes. Duplicate paths are reported once.
func Discover(rules []Rule, opts DiscoverOptions) []Target {
	type rawTarget struct {
		rule Rule
		path string
	}
	var raws []rawTarget
	for _, r := range rules {
		for _, p := range r.Paths {
			exp := fsutil.ExpandPath(p)
			if hasMeta(exp) {
				matches, err := filepath.Glob(exp)
				if err != nil {
					continue
				}
				sort.Strings(matches)
				for _, m := range matches {
					raws = append(raws, rawTarget{rule: r, path: m})
				}
				continue
			}
			raws = append(raws, rawTarget{rule: r, path: exp})
		}
	}
	seen := map[string]bool{}
	targets := make([]Target, 0, len(raws))
	for i, raw := range raws {
		cp := filepath.Clean(raw.path)
		key := cp
		if os.PathSeparator == '\\' {
			key = strings.ToLower(cp)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if opts.Progress != nil {
			opts.Progress(i+1, len(raws), cp)
		}
		t := Target{Rule: raw.rule, Path: cp}
		st, err := os.Lstat(cp)
		if err != nil {
			targets = append(targets, t)
			continue
		}
		t.Exists = true
		if st.IsDir() {
			t.IsDir = true
			sz := fsutil.DirSize(cp)
			t.Size, t.Files = sz.Bytes, sz.Files
		} else {
			t.Size = st.Size()
			t.Files = 1
		}
		// Rules are evaluated from the most specific to the most general, so a
		// path nested inside an already accepted target is already covered, and
		// a parent that only contains accepted children has nothing left to
		// delete. This keeps the reported totals free of double counting.
		nested := false
		var coveredBytes, coveredFiles int64
		for _, k := range targets {
			if !k.Exists {
				continue
			}
			if fsutil.IsUnder(cp, k.Path) && cp != k.Path {
				nested = true
				break
			}
			if fsutil.IsUnder(k.Path, cp) && k.Path != cp {
				coveredBytes += k.Size
				coveredFiles += k.Files
			}
		}
		if nested {
			continue
		}
		if coveredBytes > 0 {
			t.Size -= coveredBytes
			t.Files -= coveredFiles
			if t.Size <= 0 {
				continue
			}
		}
		targets = append(targets, t)
	}
	return targets
}

func hasMeta(p string) bool { return strings.ContainsAny(p, "*?[") }

// Item is the result of removing (or planning to remove) one target.
type Item struct {
	RuleID string `json:"rule_id"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	Files  int64  `json:"files"`
	Err    string `json:"error,omitempty"`
}

// Report summarises a cleanup run.
type Report struct {
	Removed []Item `json:"removed"`
	Failed  []Item `json:"failed"`
	Bytes   int64  `json:"bytes"`
	Files   int64  `json:"files"`
	DryRun  bool   `json:"dry_run"`
	Skipped []Item `json:"skipped,omitempty"`
}

// Apply removes every existing target. With dryRun it only reports what would
// happen. Targets that are protected or outside the filesystem are refused.
func Apply(targets []Target, dryRun bool, log func(Item)) *Report {
	rep := &Report{DryRun: dryRun}
	for _, t := range targets {
		if !t.Exists {
			rep.Skipped = append(rep.Skipped, Item{RuleID: t.Rule.ID, Path: t.Path, Err: "does not exist"})
			continue
		}
		if fsutil.IsProtected(t.Path) {
			item := Item{RuleID: t.Rule.ID, Path: t.Path, Err: "refusing to remove a protected system path"}
			rep.Failed = append(rep.Failed, item)
			if log != nil {
				log(item)
			}
			continue
		}
		item := Item{RuleID: t.Rule.ID, Path: t.Path, Bytes: t.Size, Files: t.Files}
		if dryRun {
			rep.Removed = append(rep.Removed, item)
			rep.Bytes += t.Size
			rep.Files += t.Files
			if log != nil {
				log(item)
			}
			continue
		}
		if err := removeTarget(t); err != nil {
			item.Err = err.Error()
			rep.Failed = append(rep.Failed, item)
			if log != nil {
				log(item)
			}
			continue
		}
		rep.Removed = append(rep.Removed, item)
		rep.Bytes += t.Size
		rep.Files += t.Files
		if log != nil {
			log(item)
		}
	}
	return rep
}

// removeTarget empties directories (keeping the directory itself so that
// applications do not have to recreate their layout) and deletes plain files.
func removeTarget(t Target) error {
	if !t.IsDir {
		return fsutil.ForceRemove(t.Path)
	}
	ents, err := os.ReadDir(t.Path)
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range ents {
		p := filepath.Join(t.Path, e.Name())
		if fsutil.IsProtected(p) {
			continue
		}
		if err := fsutil.ForceRemoveAll(p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Total returns the summed size of the given targets.
func Total(targets []Target) int64 {
	var sum int64
	for _, t := range targets {
		sum += t.Size
	}
	return sum
}

// AdminRules returns the rules that need administrator rights.
func AdminRules() []Rule {
	var out []Rule
	for _, r := range All() {
		if r.Admin {
			out = append(out, r.Normalized())
		}
	}
	return out
}
