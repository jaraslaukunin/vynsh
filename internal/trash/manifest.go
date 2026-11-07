package trash

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Record describes one item that vynsh moved to the trash, so that it can be
// put back later. The operating system does not expose "what was the original
// path" in a portable way, so vynsh keeps its own manifest.
type Record struct {
	Time     time.Time `json:"time"`
	Original string    `json:"original"`
	Trashed  string    `json:"trashed"`
	Bytes    int64     `json:"bytes"`
	IsDir    bool      `json:"is_dir,omitempty"`
	Restored bool      `json:"restored,omitempty"`
}

// ManifestPath returns the path of the trash manifest file.
func ManifestPath() string {
	dir, err := audit.Dir()
	if err != nil {
		return filepath.Join(fsutil.HomeDir(), ".vynsh", "trash-manifest.jsonl")
	}
	return filepath.Join(dir, "trash-manifest.jsonl")
}

// appendRecord adds one entry to the manifest. Failures are not fatal: the
// move already happened, only the ability to restore is affected.
func appendRecord(r Record) {
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	p := ManifestPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(r)
}

// Records reads the manifest, newest first, dropping entries whose trashed copy
// has disappeared (it was emptied or already restored).
func Records() ([]Record, error) {
	p := ManifestPath()
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, sc.Err()
}

// Restorable returns the manifest entries whose trashed copy still exists.
func Restorable() ([]Record, error) {
	recs, err := Records()
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		if r.Restored || r.Trashed == "" {
			continue
		}
		if !fsutil.Exists(r.Trashed) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// RestoreResult describes the outcome of restoring one record.
type RestoreResult struct {
	Record   Record `json:"record"`
	Restored string `json:"restored_to,omitempty"`
	Err      string `json:"error,omitempty"`
	DryRun   bool   `json:"dry_run"`
}

// RestoreReport aggregates restore results.
type RestoreReport struct {
	Results []RestoreResult `json:"results"`
	Bytes   int64           `json:"bytes"`
	Dry     bool            `json:"dry_run"`
}

// Restore puts the given records back where they came from. Existing files are
// never overwritten: a numbered name is used instead.
func Restore(records []Record, dryRun bool) (*RestoreReport, error) {
	rep := &RestoreReport{Dry: dryRun}
	var done []Record
	for _, r := range records {
		res := RestoreResult{Record: r, DryRun: dryRun}
		if !fsutil.Exists(r.Trashed) {
			res.Err = "no longer in the trash"
			rep.Results = append(rep.Results, res)
			continue
		}
		target := r.Original
		if fsutil.Exists(target) {
			// Keep both: restore next to the existing file.
			ext := filepath.Ext(target)
			stem := target[:len(target)-len(ext)]
			target = fsutil.UniquePath(filepath.Dir(target), filepath.Base(stem+ext))
			if target == "" {
				res.Err = "could not pick a free name"
				rep.Results = append(rep.Results, res)
				continue
			}
		}
		if dryRun {
			res.Restored = target
			rep.Results = append(rep.Results, res)
			rep.Bytes += r.Bytes
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			res.Err = err.Error()
			rep.Results = append(rep.Results, res)
			continue
		}
		if err := os.Rename(r.Trashed, target); err != nil {
			if cerr := copyTree(r.Trashed, target); cerr != nil {
				res.Err = withHint(err).Error()
				rep.Results = append(rep.Results, res)
				continue
			}
			_ = fsutil.ForceRemoveAll(r.Trashed)
		}
		res.Restored = target
		rep.Results = append(rep.Results, res)
		rep.Bytes += r.Bytes
		r.Restored = true
		done = append(done, r)
	}
	if !dryRun && len(done) > 0 {
		rewriteManifest(done)
	}
	return rep, nil
}

// rewriteManifest marks records as restored (the file is small, so it is
// rewritten in place).
func rewriteManifest(restored []Record) {
	all, err := Records()
	if err != nil {
		return
	}
	byKey := map[string]bool{}
	for _, r := range restored {
		byKey[r.Trashed] = true
	}
	for i := range all {
		if byKey[all[i].Trashed] {
			all[i].Restored = true
		}
	}
	p := ManifestPath()
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	enc := json.NewEncoder(f)
	for _, r := range all {
		_ = enc.Encode(r)
	}
	f.Close()
	_ = os.Rename(tmp, p)
}

// Describe renders a record for a list view.
func (r Record) Describe() string {
	kind := "file"
	if r.IsDir {
		kind = "dir"
	}
	return fmt.Sprintf("%s (%s, %s)", shortenHome(r.Original), kind, r.Time.Format("2006-01-02 15:04"))
}

func shortenHome(p string) string {
	if h := fsutil.HomeDir(); h != "" {
		if rel, err := filepath.Rel(h, p); err == nil && rel != ".." && !filepath.IsAbs(rel) {
			if rel == "." {
				return "~"
			}
			return "~" + string(filepath.Separator) + rel
		}
	}
	return p
}
