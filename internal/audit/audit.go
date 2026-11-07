// Package audit writes an append-only log of every destructive action vynsh
// performs, so a cleanup can always be reproduced or explained.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Entry is one audit record (JSON Lines).
type Entry struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
	Path   string    `json:"path"`
	Bytes  int64     `json:"bytes,omitempty"`
	DryRun bool      `json:"dry_run,omitempty"`
	Result string    `json:"result"`
	Error  string    `json:"error,omitempty"`
	User   string    `json:"user,omitempty"`
	Host   string    `json:"host,omitempty"`
	OS     string    `json:"os,omitempty"`
}

// Dir returns the vynsh state directory (~/.vynsh), creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = fsutil.HomeDir()
	}
	dir := filepath.Join(base, "vynsh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Path returns the audit log file path.
func Path() string {
	dir, err := Dir()
	if err != nil {
		return filepath.Join(fsutil.HomeDir(), ".vynsh", "audit.log")
	}
	return filepath.Join(dir, "audit.log")
}

// Log appends an entry to the audit log. Failure to log is not fatal.
func Log(e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.OS == "" {
		e.OS = runtime.GOOS
	}
	if e.Host == "" {
		if h, err := os.Hostname(); err == nil {
			e.Host = h
		}
	}
	if e.User == "" {
		if u := os.Getenv("USER"); u != "" {
			e.User = u
		} else if u := os.Getenv("USERNAME"); u != "" {
			e.User = u
		}
	}
	p := Path()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(e)
}

// Tail returns at most n of the most recent entries.
func Tail(n int) ([]Entry, error) {
	p := Path()
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return all, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
