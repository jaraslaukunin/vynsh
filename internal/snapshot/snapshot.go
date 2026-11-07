// Package snapshot stores scan results on disk and compares them, so a user can
// answer "what grew since last week?" without re-running a full analysis of the
// past.
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/scan"
)

// Meta describes a stored snapshot.
type Meta struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Root    string    `json:"root"`
	Depth   int       `json:"depth"`
	Total   int64     `json:"total"`
	Files   int64     `json:"files"`
	Host    string    `json:"host,omitempty"`
}

// Snapshot is a stored scan plus its metadata.
type Snapshot struct {
	Meta
	Result *scan.Result `json:"result,omitempty"`
}

// Dir returns the snapshot directory (created on demand).
func Dir() (string, error) {
	base, err := audit.Dir()
	if err != nil {
		base = filepath.Join(fsutil.HomeDir(), ".vynsh")
	}
	dir := filepath.Join(base, "snapshots")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Sanitize turns a user supplied name into a safe file name.
func Sanitize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '/' || r == ':':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// DefaultName builds a name such as "home-20261107-2115" from a root path.
func DefaultName(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "root"
	}
	return Sanitize(base) + "-" + time.Now().Format("20060102-150405")
}

// Path returns the file path for a snapshot name.
func Path(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	clean := Sanitize(name)
	if clean == "" {
		return "", fmt.Errorf("invalid snapshot name %q", name)
	}
	return filepath.Join(dir, clean+".json"), nil
}

// Save writes a scan result as a snapshot.
func Save(name string, res *scan.Result, depth int) (*Snapshot, error) {
	if res == nil {
		return nil, fmt.Errorf("nothing to save")
	}
	if name == "" {
		name = DefaultName(res.Root)
	}
	snap := &Snapshot{
		Meta: Meta{
			Name:    Sanitize(name),
			Created: time.Now(),
			Root:    res.Root,
			Depth:   depth,
			Total:   res.Total,
			Files:   res.Files,
		},
		Result: res,
	}
	snap.Host, _ = os.Hostname()
	p, err := Path(snap.Name)
	if err != nil {
		return nil, err
	}
	// Never overwrite an existing snapshot: pick the next free suffix instead.
	base := snap.Name
	for i := 2; fsutil.Exists(p); i++ {
		snap.Name = fmt.Sprintf("%s-%d", base, i)
		if p, err = Path(snap.Name); err != nil {
			return nil, err
		}
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return nil, err
	}
	return snap, nil
}

// Load reads a snapshot by name.
func Load(name string) (*Snapshot, error) {
	p, err := Path(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot %q not found (see `vynsh snapshot list`)", name)
		}
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("snapshot %q is corrupted: %w", name, err)
	}
	return &snap, nil
}

// List returns all stored snapshots, newest first.
func List() ([]Meta, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Meta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		if snap.Name == "" {
			snap.Name = strings.TrimSuffix(e.Name(), ".json")
		}
		out = append(out, snap.Meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// LatestBefore returns the newest snapshot older than the given name (or the
// newest overall when name is empty), optionally limited to one root.
func LatestBefore(name, root string) (*Meta, error) {
	metas, err := List()
	if err != nil {
		return nil, err
	}
	for _, m := range metas {
		if m.Name == name {
			continue
		}
		if root != "" && m.Root != root {
			continue
		}
		mm := m
		return &mm, nil
	}
	return nil, fmt.Errorf("no other snapshot found")
}

// Delete removes a snapshot file.
func Delete(name string) error {
	p, err := Path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Prune keeps at most n snapshots (newest kept) and returns the removed names.
func Prune(n int) ([]string, error) {
	metas, err := List()
	if err != nil {
		return nil, err
	}
	if len(metas) <= n {
		return nil, nil
	}
	var removed []string
	for _, m := range metas[n:] {
		if err := Delete(m.Name); err == nil {
			removed = append(removed, m.Name)
		}
	}
	return removed, nil
}

// Kind classifies a change between two snapshots.
type Kind string

const (
	KindGrown   Kind = "grown"
	KindShrunk  Kind = "shrunk"
	KindAdded   Kind = "added"
	KindRemoved Kind = "removed"
)

// Change describes how one directory changed.
type Change struct {
	Path       string `json:"path"`
	Kind       Kind   `json:"kind"`
	Before     int64  `json:"before"`
	After      int64  `json:"after"`
	Delta      int64  `json:"delta"`
	FilesDelta int64  `json:"files_delta"`
}

// Diff is the comparison of two snapshots.
type Diff struct {
	A          Meta     `json:"a"`
	B          Meta     `json:"b"`
	TotalDelta int64    `json:"total_delta"`
	FilesDelta int64    `json:"files_delta"`
	Changes    []Change `json:"changes"`
}

// Compare returns the differences between two snapshots, largest first.
func Compare(a, b *Snapshot) *Diff {
	d := &Diff{A: a.Meta, B: b.Meta}
	d.TotalDelta = b.Total - a.Total
	d.FilesDelta = b.Files - a.Files
	am := flatten(a.Result)
	bm := flatten(b.Result)
	seen := map[string]bool{}
	for path, an := range am {
		if path == "" {
			continue // the tree root is the total, reported separately
		}
		seen[path] = true
		bn, ok := bm[path]
		if !ok {
			d.Changes = append(d.Changes, Change{
				Path: path, Kind: KindRemoved, Before: an.Size, After: 0,
				Delta: -an.Size, FilesDelta: -an.Files,
			})
			continue
		}
		delta := bn.Size - an.Size
		if delta == 0 {
			continue
		}
		kind := KindGrown
		switch {
		case bn.Size == 0 && an.Size > 0:
			kind = KindRemoved
		case an.Size == 0 && bn.Size > 0:
			kind = KindAdded
		case delta < 0:
			kind = KindShrunk
		}
		d.Changes = append(d.Changes, Change{
			Path: path, Kind: kind, Before: an.Size, After: bn.Size,
			Delta: delta, FilesDelta: bn.Files - an.Files,
		})
	}
	for path, bn := range bm {
		if seen[path] || path == "" {
			continue
		}
		d.Changes = append(d.Changes, Change{
			Path: path, Kind: KindAdded, Before: 0, After: bn.Size,
			Delta: bn.Size, FilesDelta: bn.Files,
		})
	}
	sort.Slice(d.Changes, func(i, j int) bool {
		ai, aj := abs64(d.Changes[i].Delta), abs64(d.Changes[j].Delta)
		if ai == aj {
			return d.Changes[i].Path < d.Changes[j].Path
		}
		return ai > aj
	})
	return d
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// flatten walks a scan tree and indexes nodes by their path relative to the root.
func flatten(res *scan.Result) map[string]*scan.Node {
	out := map[string]*scan.Node{}
	if res == nil || res.Tree == nil {
		return out
	}
	var walk func(n *scan.Node)
	walk = func(n *scan.Node) {
		rel, err := filepath.Rel(res.Root, n.Path)
		if err != nil {
			rel = n.Path
		}
		if rel == "." {
			rel = ""
		}
		out[rel] = n
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(res.Tree)
	return out
}
