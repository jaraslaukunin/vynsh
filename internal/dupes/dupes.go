// Package dupes finds duplicate files: it filters by size, confirms with a
// cheap partial hash and only then computes a full SHA-256, so large trees are
// scanned without reading every byte twice.
package dupes

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Options controls the duplicate search.
type Options struct {
	MinSize       int64
	MaxSize       int64
	IncludeHidden bool
	Exclude       []string
	Progress      func(phase string, done, total int64)
}

// Group is a set of identical files.
type Group struct {
	Size  int64    `json:"size"`
	Hash  string   `json:"hash"`
	Paths []string `json:"paths"`
}

// Wasted returns the bytes reclaimable by keeping a single copy.
func (g Group) Wasted() int64 {
	if len(g.Paths) < 2 {
		return 0
	}
	return int64(len(g.Paths)-1) * g.Size
}

// Result summarises a duplicate search.
type Result struct {
	Roots        []string `json:"roots"`
	FilesScanned int64    `json:"files_scanned"`
	Candidates   int64    `json:"candidates"`
	Groups       []Group  `json:"groups"`
	Wasted       int64    `json:"wasted"`
	Elapsed      float64  `json:"elapsed_seconds"`
	Errors       []string `json:"errors,omitempty"`
}

type fileInfo struct {
	path string
	size int64
}

// Find searches roots for duplicate files.
func Find(roots []string, opts Options) (*Result, error) {
	start := time.Now()
	res := &Result{Roots: roots}
	if opts.MinSize <= 0 {
		opts.MinSize = 1
	}
	// Phase 1: group by size.
	bySize := map[int64][]fileInfo{}
	var scanned int64
	for _, raw := range roots {
		root := fsutil.ExpandPath(raw)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				res.Errors = append(res.Errors, p+": "+err.Error())
				if d != nil && d.IsDir() && p != root {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			name := d.Name()
			if !opts.IncludeHidden && p != root && len(name) > 0 && name[0] == '.' {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			for _, ex := range opts.Exclude {
				if ok, _ := filepath.Match(ex, p); ok {
					if d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			size := info.Size()
			if size < opts.MinSize {
				return nil
			}
			if opts.MaxSize > 0 && size > opts.MaxSize {
				return nil
			}
			scanned++
			bySize[size] = append(bySize[size], fileInfo{path: p, size: size})
			if opts.Progress != nil && scanned%500 == 0 {
				opts.Progress("scan", scanned, 0)
			}
			return nil
		})
		if err != nil {
			res.Errors = append(res.Errors, root+": "+err.Error())
		}
	}
	res.FilesScanned = scanned
	if opts.Progress != nil {
		opts.Progress("scan", scanned, scanned)
	}

	var candidates []fileInfo
	for _, files := range bySize {
		if len(files) > 1 {
			candidates = append(candidates, files...)
		}
	}
	res.Candidates = int64(len(candidates))
	if len(candidates) == 0 {
		res.Elapsed = time.Since(start).Seconds()
		return res, nil
	}

	// Phase 2: partial hash (first 64 KiB).
	byPartial := map[string][]fileInfo{}
	var hashed int64
	for _, f := range candidates {
		h, err := hashFile(f.path, 64*1024, nil)
		if err != nil {
			res.Errors = append(res.Errors, f.path+": "+err.Error())
			continue
		}
		key := itoa(f.size) + ":" + h
		byPartial[key] = append(byPartial[key], f)
		hashed++
		if opts.Progress != nil && hashed%100 == 0 {
			opts.Progress("partial hash", hashed, res.Candidates)
		}
	}
	if opts.Progress != nil {
		opts.Progress("partial hash", hashed, res.Candidates)
	}

	var fullCandidates []fileInfo
	for _, files := range byPartial {
		if len(files) > 1 {
			fullCandidates = append(fullCandidates, files...)
		}
	}

	// Phase 3: full SHA-256.
	byFull := map[string][]fileInfo{}
	var fullDone int64
	for _, f := range fullCandidates {
		h, err := hashFile(f.path, 0, sha256.New())
		if err != nil {
			res.Errors = append(res.Errors, f.path+": "+err.Error())
			continue
		}
		key := itoa(f.size) + ":" + h
		byFull[key] = append(byFull[key], f)
		fullDone++
		if opts.Progress != nil && fullDone%20 == 0 {
			opts.Progress("full hash", fullDone, int64(len(fullCandidates)))
		}
	}
	if opts.Progress != nil {
		opts.Progress("full hash", fullDone, int64(len(fullCandidates)))
	}

	for key, files := range byFull {
		if len(files) < 2 {
			continue
		}
		size := files[0].size
		paths := make([]string, 0, len(files))
		for _, f := range files {
			paths = append(paths, f.path)
		}
		sort.Strings(paths)
		hashPart := key
		if i := indexByte(key, ':'); i >= 0 {
			hashPart = key[i+1:]
		}
		g := Group{Size: size, Hash: hashPart, Paths: paths}
		res.Groups = append(res.Groups, g)
		res.Wasted += g.Wasted()
	}
	sort.Slice(res.Groups, func(i, j int) bool {
		wi, wj := res.Groups[i].Wasted(), res.Groups[j].Wasted()
		if wi == wj {
			return res.Groups[i].Size > res.Groups[j].Size
		}
		return wi > wj
	})
	res.Elapsed = time.Since(start).Seconds()
	return res, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// hashFile returns a hex digest of the file. limit > 0 hashes only the leading
// bytes; a nil hasher means a plain SHA-256 of the prefix.
func hashFile(path string, limit int64, hasher io.Writer) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if hasher == nil {
		h := sha256.New()
		var r io.Reader = f
		if limit > 0 {
			r = io.LimitReader(f, limit)
		}
		if _, err := io.Copy(h, r); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	h, ok := hasher.(interface {
		io.Writer
		Sum([]byte) []byte
	})
	if !ok {
		return "", os.ErrInvalid
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
