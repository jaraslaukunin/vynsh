// Package del implements selective deletion: it builds a plan from filter
// criteria, shows it, and only then removes files (optionally to the trash).
// Directories are never removed recursively — only files, plus directories that
// ended up empty, so a pattern can never silently swallow unflagged content.
package del

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/trash"
)

// Filter describes which entries are selected for deletion.
type Filter struct {
	Names         []string      // glob patterns matched against the base name
	ExcludeNames  []string      // glob patterns never deleted
	Exts          []string      // extensions, with or without the leading dot
	OlderThan     time.Duration // mtime must be older than now-OlderThan
	NewerThan     time.Duration // mtime must be newer than now-NewerThan
	MinSize       int64
	MaxSize       int64
	Files         bool
	Dirs          bool // also delete directories that are empty
	IncludeHidden bool
	MaxDepth      int // 0 = unlimited
}

// DefaultFilter returns a filter that selects every regular file.
func DefaultFilter() Filter { return Filter{Files: true} }

// Item is a single planned deletion.
type Item struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

// Plan is the outcome of evaluating roots against a filter.
type Plan struct {
	Roots   []string `json:"roots"`
	Items   []Item   `json:"items"`
	Bytes   int64    `json:"bytes"`
	Files   int64    `json:"files"`
	Dirs    int64    `json:"dirs"`
	Skipped int64    `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// Build walks roots and collects the entries matching f.
func Build(roots []string, f Filter, progress func(scanned int64, path string)) (*Plan, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("no paths given")
	}
	plan := &Plan{}
	now := time.Now()
	for _, raw := range roots {
		expanded, err := expandRoot(raw)
		if err != nil {
			plan.Errors = append(plan.Errors, fmt.Sprintf("%s: %v", raw, err))
			continue
		}
		for _, root := range expanded {
			plan.Roots = append(plan.Roots, root)
			st, err := os.Lstat(root)
			if err != nil {
				plan.Errors = append(plan.Errors, fmt.Sprintf("%s: %v", root, err))
				continue
			}
			if !st.IsDir() {
				if f.match(root, st, now) {
					plan.add(Item{Path: root, Size: st.Size(), ModTime: st.ModTime()})
				} else {
					plan.Skipped++
				}
				continue
			}
			baseDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
			walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					plan.Errors = append(plan.Errors, fmt.Sprintf("%s: %v", p, err))
					if d != nil && d.IsDir() && p != root {
						return fs.SkipDir
					}
					return nil
				}
				if p == root {
					return nil
				}
				name := d.Name()
				// Never follow symlinks; they are skipped entirely.
				if d.Type()&os.ModeSymlink != 0 {
					return nil
				}
				if f.MaxDepth > 0 {
					depth := strings.Count(filepath.Clean(p), string(os.PathSeparator)) - baseDepth
					if depth > f.MaxDepth {
						if d.IsDir() {
							return fs.SkipDir
						}
						return nil
					}
				}
				info, ierr := d.Info()
				if ierr != nil {
					return nil
				}
				if d.IsDir() {
					if !f.IncludeHidden && isHidden(name) {
						return fs.SkipDir
					}
					if f.Dirs && f.matchDir(p, info, now) {
						plan.add(Item{Path: p, IsDir: true, ModTime: info.ModTime()})
					}
					return nil
				}
				if progress != nil {
					progress(plan.Files+plan.Skipped+1, p)
				}
				if !info.Mode().IsRegular() {
					return nil
				}
				if f.match(p, info, now) {
					plan.add(Item{Path: p, Size: info.Size(), ModTime: info.ModTime()})
				} else {
					plan.Skipped++
				}
				return nil
			})
			if walkErr != nil && walkErr != fs.SkipAll {
				plan.Errors = append(plan.Errors, fmt.Sprintf("%s: %v", root, walkErr))
			}
		}
	}
	// Deepest paths first so that directories are removed after their content.
	sort.SliceStable(plan.Items, func(i, j int) bool {
		return len(plan.Items[i].Path) > len(plan.Items[j].Path)
	})
	return plan, nil
}

func (p *Plan) add(it Item) {
	p.Items = append(p.Items, it)
	if it.IsDir {
		p.Dirs++
		return
	}
	p.Files++
	p.Bytes += it.Size
}

// expandRoot resolves ~, env vars and globs. A plain directory is returned as-is.
func expandRoot(raw string) ([]string, error) {
	p := fsutil.ExpandPath(raw)
	if strings.ContainsAny(p, "*?[") {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no matches")
		}
		sort.Strings(matches)
		return matches, nil
	}
	if _, err := os.Lstat(p); err != nil {
		return nil, err
	}
	return []string{p}, nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(strings.ToLower(name), "desktop.ini")
}

// match evaluates a non-directory entry against the filter.
func (f Filter) match(path string, info os.FileInfo, now time.Time) bool {
	if fsutil.IsProtected(path) {
		return false
	}
	name := filepath.Base(path)
	if !f.IncludeHidden && isHidden(name) {
		return false
	}
	if !f.Files || !info.Mode().IsRegular() {
		return false
	}
	if len(f.Names) > 0 || len(f.Exts) > 0 {
		okName := false
		for _, pat := range f.Names {
			if globMatch(pat, name) {
				okName = true
				break
			}
		}
		if len(f.Names) == 0 {
			okName = true
		}
		okExt := false
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		for _, e := range f.Exts {
			e = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e)), ".")
			if e == ext && e != "" {
				okExt = true
				break
			}
		}
		if len(f.Exts) == 0 {
			okExt = true
		}
		if !okName || !okExt {
			return false
		}
	}
	for _, ex := range f.ExcludeNames {
		if globMatch(ex, name) {
			return false
		}
	}
	if f.OlderThan > 0 && now.Sub(info.ModTime()) < f.OlderThan {
		return false
	}
	if f.NewerThan > 0 && now.Sub(info.ModTime()) > f.NewerThan {
		return false
	}
	if f.MinSize > 0 && info.Size() < f.MinSize {
		return false
	}
	if f.MaxSize > 0 && info.Size() > f.MaxSize {
		return false
	}
	return true
}

// matchDir evaluates a directory: only empty directories are ever removed.
func (f Filter) matchDir(path string, info os.FileInfo, now time.Time) bool {
	if fsutil.IsProtected(path) {
		return false
	}
	ents, err := os.ReadDir(path)
	if err != nil || len(ents) > 0 {
		return false
	}
	if f.OlderThan > 0 && now.Sub(info.ModTime()) < f.OlderThan {
		return false
	}
	for _, pat := range f.Names {
		if globMatch(pat, filepath.Base(path)) {
			return true
		}
	}
	return len(f.Names) == 0
}

func globMatch(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	pat := pattern
	nm := name
	if os.PathSeparator == '\\' {
		pat = strings.ToLower(pat)
		nm = strings.ToLower(nm)
	}
	if ok, err := filepath.Match(pat, nm); err == nil && ok {
		return true
	}
	return strings.EqualFold(pattern, name)
}

// ApplyOptions controls the deletion step.
type ApplyOptions struct {
	DryRun  bool
	ToTrash bool              // move to the recycle bin instead of unlinking
	Log     func(Item, error) // audit hook, may be nil
	Limit   int64             // 0 = unlimited
	// PruneDirs removes directories that became empty after the files were
	// deleted (the --empty-dirs behaviour).
	PruneDirs bool
}

// Result summarises a deletion run.
type Result struct {
	Deleted []Item   `json:"deleted"`
	Failed  []Item   `json:"failed"`
	Bytes   int64    `json:"bytes"`
	Files   int64    `json:"files"`
	Dirs    int64    `json:"dirs"`
	DryRun  bool     `json:"dry_run"`
	ToTrash bool     `json:"to_trash"`
	Limited bool     `json:"limited"`
	Errors  []string `json:"errors,omitempty"`
}

// Apply deletes the planned items. Empty-directory items are re-checked right
// before removal so that a race never deletes content that appeared meanwhile.
func Apply(plan *Plan, opts ApplyOptions) *Result {
	res := &Result{DryRun: opts.DryRun, ToTrash: opts.ToTrash}
	count := int64(0)
	for _, it := range plan.Items {
		if opts.Limit > 0 && count >= opts.Limit {
			res.Limited = true
			break
		}
		if fsutil.IsProtected(it.Path) {
			res.Failed = append(res.Failed, it)
			res.Errors = append(res.Errors, it.Path+": protected path")
			if opts.Log != nil {
				opts.Log(it, fmt.Errorf("protected path"))
			}
			continue
		}
		var err error
		if it.IsDir {
			var ents []os.DirEntry
			ents, err = os.ReadDir(it.Path)
			if err == nil && len(ents) > 0 {
				continue // no longer empty; leave it alone
			}
		}
		if !opts.DryRun {
			if opts.ToTrash {
				rep, terr := trash.Move([]string{it.Path}, false)
				if terr != nil {
					err = terr
				} else if len(rep.Moved) > 0 && rep.Moved[0].Err != "" {
					err = fmt.Errorf("%s", rep.Moved[0].Err)
				}
			} else {
				err = fsutil.ForceRemove(it.Path)
			}
		}
		if err != nil {
			res.Failed = append(res.Failed, it)
			res.Errors = append(res.Errors, it.Path+": "+err.Error())
			if opts.Log != nil {
				opts.Log(it, err)
			}
			continue
		}
		count++
		res.Deleted = append(res.Deleted, it)
		if it.IsDir {
			res.Dirs++
		} else {
			res.Files++
			res.Bytes += it.Size
		}
		if opts.Log != nil {
			opts.Log(it, nil)
		}
	}
	if opts.PruneDirs && !opts.DryRun && res.Files > 0 {
		for _, root := range plan.Roots {
			if fsutil.IsProtected(root) {
				continue
			}
			for _, it := range pruneEmptyTree(root) {
				res.Deleted = append(res.Deleted, it)
				res.Dirs++
				if opts.Log != nil {
					opts.Log(it, nil)
				}
			}
		}
	}
	return res
}

// pruneEmptyTree removes directories that are empty, bottom-up, and returns
// what was removed. The root itself is removed only when it ends up empty too.
func pruneEmptyTree(root string) []Item {
	var removed []Item
	var walk func(dir string) bool // reports whether dir is empty (and gone)
	walk = func(dir string) bool {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		empty := true
		for _, e := range ents {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if !walk(p) {
					empty = false
				}
				continue
			}
			empty = false
		}
		if !empty || fsutil.IsProtected(dir) {
			return false
		}
		if err := fsutil.ForceRemove(dir); err != nil {
			return false
		}
		removed = append(removed, Item{Path: dir, IsDir: true})
		return true
	}
	if fsutil.Exists(root) {
		walk(root)
	}
	return removed
}
