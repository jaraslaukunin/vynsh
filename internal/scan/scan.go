// Package scan implements the disk usage analyser: it walks a directory tree
// once, aggregating sizes per directory (bounded by a display depth), and
// collects the largest files, file-type statistics and volume usage.
//
// The walk is parallel: directories are visited by a bounded pool of workers so
// that a cold cache on a large tree stays usable. Symbolic links and Windows
// junctions are never followed.
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Options controls a scan run.
type Options struct {
	Root      string   // directory or file to analyse
	Depth     int      // directory depth materialised in the tree (0 = root only)
	MinSize   int64    // nodes smaller than this are hidden in the report
	TopFiles  int      // how many of the largest files to collect
	TopDirs   int      // how many large directories to collect
	Exclude   []string // extra paths or glob patterns to skip
	Progress  func(files int64, bytes int64)
	NoDefault bool // do not apply the built-in exclusion list
	Workers   int  // parallelism (0 = number of CPUs)
}

// Node is one entry of the aggregated directory tree.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Files    int64   `json:"files"`
	IsDir    bool    `json:"is_dir"`
	Children []*Node `json:"children,omitempty"`
}

// FileEntry describes a single large file.
type FileEntry struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// ExtStat aggregates usage by file extension.
type ExtStat struct {
	Ext   string `json:"ext"`
	Size  int64  `json:"size"`
	Count int64  `json:"count"`
}

// Skip records a path that could not be read.
type Skip struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

// Volume describes the filesystem holding the scanned root.
type Volume struct {
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
	Free   uint64 `json:"free"`
	FSType string `json:"fs_type"`
}

// Result is the outcome of a scan.
type Result struct {
	Root     string      `json:"root"`
	Total    int64       `json:"total"`
	Files    int64       `json:"files"`
	Dirs     int64       `json:"dirs"`
	Elapsed  float64     `json:"elapsed_seconds"`
	Tree     *Node       `json:"tree"`
	TopFiles []FileEntry `json:"top_files"`
	TopDirs  []*Node     `json:"top_dirs"`
	ByExt    []ExtStat   `json:"by_ext"`
	Skipped  []Skip      `json:"skipped,omitempty"`
	Volume   *Volume     `json:"volume,omitempty"`
}

// DefaultExcludes returns paths that would make a scan useless or duplicate
// data: on macOS "/System/Volumes" re-exports the whole data volume, and on
// Windows the recycle bin and volume metadata are not user data.
func DefaultExcludes() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`*\System Volume Information*`,
			`*\$Recycle.Bin*`,
			`*\Windows\WinSxS`,
		}
	}
	return []string{
		"/System/Volumes",
		"/Volumes",
		"/dev",
		"/private/var/vm",
		"/.Spotlight-V100",
		"/.fseventsd",
		"/.DocumentRevisions-V100",
	}
}

// topN keeps the largest N items using a bounded slice.
type topN struct {
	n     int
	items []FileEntry
}

func (t *topN) add(e FileEntry) {
	if t.n <= 0 {
		return
	}
	if len(t.items) < t.n {
		t.items = append(t.items, e)
		if len(t.items) == t.n {
			t.sortAndTrim()
		}
		return
	}
	if len(t.items) > 0 && e.Size <= t.items[len(t.items)-1].Size {
		return
	}
	t.items = append(t.items, e)
	if len(t.items) >= t.n*2 {
		t.sortAndTrim()
	}
}

func (t *topN) sortAndTrim() {
	sort.Slice(t.items, func(i, j int) bool { return t.items[i].Size > t.items[j].Size })
	if len(t.items) > t.n {
		t.items = t.items[:t.n]
	}
}

func (t *topN) result() []FileEntry {
	t.sortAndTrim()
	out := make([]FileEntry, len(t.items))
	copy(out, t.items)
	return out
}

// state carries the shared, mutex protected accumulators of one scan.
type state struct {
	opts     Options
	depth    int
	excludes []string
	sem      chan struct{}
	done     chan struct{}

	mu       sync.Mutex
	fileSink *topN
	exts     map[string]*ExtStat
	dirs     []*Node
	skipped  []Skip
	skipN    int

	files    atomic.Int64
	bytes    atomic.Int64
	lastTick atomic.Int64

	linksMu sync.Mutex
	links   map[[2]uint64]bool
}

// pending is one directory being visited. It aggregates the sizes of its files
// and of its child directories, and propagates the totals upwards once every
// child has reported.
type pending struct {
	path   string
	name   string
	level  int
	node   *Node
	parent *pending

	onComplete func()

	mu      sync.Mutex
	pending int
	size    int64
	files   int64
}

// Run performs the scan.
func Run(opts Options) (*Result, error) {
	if opts.Root == "" {
		opts.Root = "."
	}
	opts.Root = fsutil.ExpandPath(opts.Root)
	if opts.Depth <= 0 {
		opts.Depth = 1
	}
	if opts.TopFiles <= 0 {
		opts.TopFiles = 20
	}
	if opts.TopDirs <= 0 {
		opts.TopDirs = 15
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > 32 {
		workers = 32
	}
	st, err := os.Stat(opts.Root)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	res := &Result{Root: opts.Root}
	if vol := volumeInfo(opts.Root); vol != nil {
		res.Volume = vol
	}
	if !st.IsDir() {
		res.Total = st.Size()
		res.Files = 1
		res.Tree = &Node{Name: filepath.Base(opts.Root), Path: opts.Root, Size: st.Size(), Files: 1}
		res.TopFiles = []FileEntry{{Path: opts.Root, Size: st.Size(), ModTime: st.ModTime()}}
		res.Elapsed = time.Since(start).Seconds()
		return res, nil
	}

	s := &state{
		opts:     opts,
		depth:    opts.Depth,
		sem:      make(chan struct{}, workers*2),
		done:     make(chan struct{}),
		fileSink: &topN{n: opts.TopFiles},
		exts:     make(map[string]*ExtStat),
		links:    make(map[[2]uint64]bool),
	}
	s.lastTick.Store(start.UnixNano())
	if !opts.NoDefault {
		s.excludes = append(s.excludes, DefaultExcludes()...)
	}
	s.excludes = append(s.excludes, opts.Exclude...)

	rootNode := &Node{Name: displayName(opts.Root), Path: opts.Root, IsDir: true}
	root := &pending{path: opts.Root, name: rootNode.Name, level: 0, node: rootNode, pending: 1}
	root.onComplete = func() { close(s.done) }
	s.process(root)
	<-s.done

	res.Total = rootNode.Size
	res.Files = rootNode.Files
	res.Tree = rootNode
	res.Dirs = rootNode.dirs()
	res.TopFiles = s.fileSink.result()
	res.ByExt = s.topExts(15)
	res.Skipped = s.skipped
	res.TopDirs = s.topDirs(opts.TopDirs)
	res.Elapsed = time.Since(start).Seconds()
	if opts.Progress != nil {
		opts.Progress(rootNode.Files, rootNode.Size)
	}
	return res, nil
}

// dirs counts every directory below the root (the root itself excluded).
func (n *Node) dirs() int64 {
	var total int64
	for _, c := range n.Children {
		total += 1 + c.dirs()
	}
	return total
}

func displayName(root string) string {
	if root == string(filepath.Separator) {
		return root
	}
	return filepath.Base(root)
}

// spawn runs fn on a pooled goroutine, or inline when the pool is saturated.
// Running inline keeps the number of live goroutines bounded and can never
// deadlock because the parent never waits for a child.
func (s *state) spawn(fn func()) {
	select {
	case s.sem <- struct{}{}:
		go func() {
			defer func() { <-s.sem }()
			fn()
		}()
	default:
		fn()
	}
}

// process reads one directory and schedules its subdirectories.
func (s *state) process(p *pending) {
	ents, err := os.ReadDir(p.path)
	if err != nil {
		s.addSkip(p.path, err)
		p.selfDone()
		return
	}
	var localSize, localFiles int64
	for _, e := range ents {
		name := e.Name()
		childPath := filepath.Join(p.path, name)
		if s.isExcluded(childPath) {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&os.ModeSymlink != 0 {
			continue // never follow links
		}
		var info fs.FileInfo
		if !isDir {
			var ierr error
			info, ierr = e.Info()
			if ierr != nil {
				s.addSkip(childPath, ierr)
				continue
			}
			isDir = info.IsDir()
		}
		if isDir {
			child := &pending{path: childPath, name: name, level: p.level + 1, parent: p, pending: 1}
			if p.level+1 <= s.depth {
				child.node = &Node{Name: name, Path: childPath, IsDir: true}
				s.mu.Lock()
				if p.node != nil {
					p.node.Children = append(p.node.Children, child.node)
				}
				s.dirs = append(s.dirs, child.node)
				s.mu.Unlock()
			}
			p.mu.Lock()
			p.pending++
			p.mu.Unlock()
			s.spawn(func() { s.process(child) })
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		size := info.Size()
		localSize += size
		localFiles++
		s.noteFile(childPath, name, info)
	}
	p.addFiles(localSize, localFiles)
	p.selfDone()
}

// addFiles records the sizes of the files stored directly in p.
func (p *pending) addFiles(size, files int64) {
	p.mu.Lock()
	p.size += size
	p.files += files
	p.mu.Unlock()
}

// selfDone reports that p has finished reading its own entries.
func (p *pending) selfDone() {
	p.mu.Lock()
	p.pending--
	done := p.pending == 0
	size, files := p.size, p.files
	p.mu.Unlock()
	if done {
		p.complete(size, files)
	}
}

// complete publishes the totals of p and hands them to the parent.
func (p *pending) complete(size, files int64) {
	if p.node != nil {
		p.node.Size = size
		p.node.Files = files
	}
	if p.parent == nil {
		if p.onComplete != nil {
			p.onComplete()
		}
		return
	}
	q := p.parent
	q.mu.Lock()
	q.size += size
	q.files += files
	q.pending--
	done := q.pending == 0
	qs, qf := q.size, q.files
	q.mu.Unlock()
	if done {
		q.complete(qs, qf)
	}
}

// noteFile feeds the shared file, extension and progress accumulators. Hard
// linked files are counted once, like du(1) does.
func (s *state) noteFile(path, name string, info fs.FileInfo) {
	size := info.Size()
	mod := info.ModTime()
	if dev, ino, nlink, ok := fsutil.FileID(info); ok && nlink > 1 {
		key := [2]uint64{dev, ino}
		s.linksMu.Lock()
		seen := s.links[key]
		if !seen {
			s.links[key] = true
		}
		s.linksMu.Unlock()
		if seen {
			return
		}
	}
	s.mu.Lock()
	s.fileSink.add(FileEntry{Path: path, Size: size, ModTime: mod})
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		ext = "(none)"
	}
	es := s.exts[ext]
	if es == nil {
		es = &ExtStat{Ext: ext}
		s.exts[ext] = es
	}
	es.Size += size
	es.Count++
	s.mu.Unlock()

	files := s.files.Add(1)
	s.bytes.Add(size)
	if s.opts.Progress != nil && files%2048 == 0 {
		now := time.Now().UnixNano()
		last := s.lastTick.Load()
		if now-last > int64(120*time.Millisecond) && s.lastTick.CompareAndSwap(last, now) {
			s.opts.Progress(files, s.bytes.Load())
		}
	}
}

func (s *state) isExcluded(p string) bool {
	cp := filepath.Clean(p)
	lower := cp
	if runtime.GOOS == "windows" {
		lower = strings.ToLower(cp)
	}
	for _, ex := range s.excludes {
		if ex == "" {
			continue
		}
		if strings.ContainsAny(ex, "*?[") {
			if ok, _ := filepath.Match(ex, p); ok {
				return true
			}
			plain := strings.NewReplacer("*", "", "?", "").Replace(ex)
			if plain != "" && strings.Contains(lower, strings.ToLower(plain)) {
				return true
			}
			continue
		}
		exAbs := fsutil.ExpandPath(ex)
		exLower := exAbs
		if runtime.GOOS == "windows" {
			exLower = strings.ToLower(exAbs)
		}
		if lower == exLower || fsutil.IsUnder(lower, exLower) {
			return true
		}
	}
	return false
}

func (s *state) addSkip(path string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipN++
	if len(s.skipped) >= 25 {
		return
	}
	s.skipped = append(s.skipped, Skip{Path: path, Err: err.Error()})
}

func (s *state) topExts(n int) []ExtStat {
	out := make([]ExtStat, 0, len(s.exts))
	for _, e := range s.exts {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size == out[j].Size {
			return out[i].Count > out[j].Count
		}
		return out[i].Size > out[j].Size
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (s *state) topDirs(n int) []*Node {
	out := make([]*Node, 0, len(s.dirs))
	for _, d := range s.dirs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func volumeInfo(root string) *Volume {
	u, err := disk.Usage(root)
	if err != nil || u == nil {
		return nil
	}
	return &Volume{Total: u.Total, Used: u.Used, Free: u.Free, FSType: u.Fstype}
}
