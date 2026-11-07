// Package sysinfo collects live resource usage: CPU, memory, swap, disks,
// network throughput and the busiest processes. It is stateful so that
// successive snapshots can report real rates instead of averages since boot.
package sysinfo

import (
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Options controls snapshot collection.
type Options struct {
	Interval     time.Duration // CPU sampling window
	TopN         int           // number of processes to keep
	SortBy       string        // "cpu" or "mem"
	IncludeProcs bool
	IncludeUser  bool
}

// Load holds the 1/5/15 minute load averages (zero on platforms without them).
type Load struct {
	L1  float64 `json:"load1"`
	L5  float64 `json:"load5"`
	L15 float64 `json:"load15"`
}

// CPUInfo describes processor utilisation.
type CPUInfo struct {
	Model   string    `json:"model"`
	Cores   int       `json:"cores"`
	Percent float64   `json:"percent"`
	PerCore []float64 `json:"per_core"`
	Load    Load      `json:"load"`
}

// MemInfo describes memory and swap pressure.
type MemInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	Percent     float64 `json:"percent"`
	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapPercent float64 `json:"swap_percent"`
}

// DiskInfo describes one mounted filesystem.
type DiskInfo struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	FSType     string  `json:"fs_type"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Percent    float64 `json:"percent"`
}

// NetInfo describes one network interface.
type NetInfo struct {
	Name     string  `json:"name"`
	Sent     uint64  `json:"sent"`
	Recv     uint64  `json:"recv"`
	SendRate float64 `json:"send_rate"`
	RecvRate float64 `json:"recv_rate"`
}

// Proc describes one running process.
type Proc struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	User       string  `json:"user,omitempty"`
	CPU        float64 `json:"cpu_percent"`
	RSS        uint64  `json:"rss"`
	MemPercent float64 `json:"mem_percent"`
	Status     string  `json:"status"`
	Threads    int32   `json:"threads"`
}

// Snapshot is a full resource usage sample.
type Snapshot struct {
	Time      time.Time  `json:"time"`
	Hostname  string     `json:"hostname"`
	OS        string     `json:"os"`
	Platform  string     `json:"platform"`
	Kernel    string     `json:"kernel"`
	Arch      string     `json:"arch"`
	Uptime    uint64     `json:"uptime"`
	CPU       CPUInfo    `json:"cpu"`
	Mem       MemInfo    `json:"mem"`
	Disks     []DiskInfo `json:"disks"`
	Net       []NetInfo  `json:"net"`
	Procs     []Proc     `json:"procs,omitempty"`
	ProcCount int        `json:"proc_count"`
}

// Monitor keeps the state needed to compute deltas between snapshots.
type Monitor struct {
	opts      Options
	mu        sync.Mutex
	prevProc  map[int32]float64
	prevNet   map[string][2]uint64
	prevAt    time.Time
	userCache map[uint32]string
	coreCount int
}

// NewMonitor creates a monitor; the same monitor should be reused across
// snapshots so that process and network rates are accurate.
func NewMonitor(opts Options) *Monitor {
	if opts.Interval <= 0 {
		opts.Interval = 200 * time.Millisecond
	}
	if opts.TopN <= 0 {
		opts.TopN = 10
	}
	if opts.SortBy == "" {
		opts.SortBy = "cpu"
	}
	ncpu := runtime.NumCPU()
	if n, err := cpu.Counts(true); err == nil && n > 0 {
		ncpu = n
	}
	return &Monitor{
		opts:      opts,
		prevProc:  map[int32]float64{},
		prevNet:   map[string][2]uint64{},
		userCache: map[uint32]string{},
		coreCount: ncpu,
	}
}

// Collect takes one snapshot. It blocks for roughly Options.Interval because
// CPU and network percentages require two samples.
func (m *Monitor) Collect() (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap := &Snapshot{Time: time.Now(), Arch: runtime.GOOS + "/" + runtime.GOARCH}
	if hi, err := host.Info(); err == nil {
		snap.Hostname = hi.Hostname
		snap.OS = hi.OS
		snap.Platform = strings.TrimSpace(hi.Platform + " " + hi.PlatformVersion)
		snap.Kernel = hi.KernelVersion
		snap.Uptime = hi.Uptime
	}

	t0 := time.Now()
	proc0 := map[int32]float64{}
	if m.opts.IncludeProcs {
		proc0 = m.sampleProcTimes()
	}
	net0 := m.sampleNet()

	perCore, err := cpu.Percent(m.opts.Interval, true)
	if err != nil {
		perCore = nil
	}
	t1 := time.Now()
	elapsed := t1.Sub(t0).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}

	snap.CPU.PerCore = perCore
	snap.CPU.Cores = len(perCore)
	if snap.CPU.Cores == 0 {
		snap.CPU.Cores = m.coreCount
	}
	if len(perCore) > 0 {
		var sum float64
		for _, v := range perCore {
			sum += v
		}
		snap.CPU.Percent = sum / float64(len(perCore))
	} else if v, err := cpu.Percent(0, false); err == nil && len(v) > 0 {
		snap.CPU.Percent = v[0]
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		snap.CPU.Model = strings.TrimSpace(infos[0].ModelName)
	}
	if avg, err := load.Avg(); err == nil && avg != nil {
		snap.CPU.Load = Load{L1: avg.Load1, L5: avg.Load5, L15: avg.Load15}
	}

	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		snap.Mem.Total = vm.Total
		snap.Mem.Free = vm.Free
		snap.Mem.Available = vm.Available
		if vm.Available > 0 {
			snap.Mem.Used = vm.Total - vm.Available
		} else {
			snap.Mem.Used = vm.Used
		}
		snap.Mem.Percent = percent(snap.Mem.Used, snap.Mem.Total)
	}
	if sw, err := mem.SwapMemory(); err == nil && sw != nil {
		snap.Mem.SwapTotal = sw.Total
		snap.Mem.SwapUsed = sw.Used
		snap.Mem.SwapPercent = sw.UsedPercent
	}

	snap.Disks = collectDisks()

	net1 := m.sampleNet()
	for name, c1 := range net1 {
		c0, ok := net0[name]
		if !ok {
			continue
		}
		sendRate := float64(c1[0]-c0[0]) / elapsed
		recvRate := float64(c1[1]-c0[1]) / elapsed
		if c1[0] < c0[0] {
			sendRate = 0
		}
		if c1[1] < c0[1] {
			recvRate = 0
		}
		if sendRate == 0 && recvRate == 0 && c1[0] == 0 && c1[1] == 0 {
			continue
		}
		snap.Net = append(snap.Net, NetInfo{Name: name, Sent: c1[0], Recv: c1[1], SendRate: sendRate, RecvRate: recvRate})
	}
	sort.Slice(snap.Net, func(i, j int) bool {
		return snap.Net[i].SendRate+snap.Net[i].RecvRate > snap.Net[j].SendRate+snap.Net[j].RecvRate
	})
	m.prevNet = net1

	if m.opts.IncludeProcs {
		procs, total := m.collectProcs(proc0, elapsed)
		snap.Procs = procs
		snap.ProcCount = total
	}
	return snap, nil
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func (m *Monitor) sampleNet() map[string][2]uint64 {
	out := map[string][2]uint64{}
	counters, err := gopsnet.IOCounters(true)
	if err != nil {
		return out
	}
	for _, c := range counters {
		if c.Name == "lo0" || c.Name == "lo" || strings.HasPrefix(c.Name, "Loopback") {
			continue
		}
		out[c.Name] = [2]uint64{c.BytesSent, c.BytesRecv}
	}
	return out
}

func (m *Monitor) sampleProcTimes() map[int32]float64 {
	out := map[int32]float64{}
	procs, err := process.Processes()
	if err != nil {
		return out
	}
	for _, p := range procs {
		ts, err := p.Times()
		if err != nil || ts == nil {
			continue
		}
		out[p.Pid] = ts.Total()
	}
	return out
}

func (m *Monitor) collectProcs(proc0 map[int32]float64, elapsed float64) ([]Proc, int) {
	procs, err := process.Processes()
	if err != nil {
		return nil, 0
	}
	out := make([]Proc, 0, len(procs))
	for _, p := range procs {
		pi := Proc{PID: p.Pid}
		if name, err := p.Name(); err == nil {
			pi.Name = name
		}
		if pi.Name == "" {
			pi.Name = "?"
		}
		if t1, err := p.Times(); err == nil && t1 != nil {
			if t0, ok := proc0[p.Pid]; ok {
				delta := t1.Total() - t0
				if delta < 0 {
					delta = 0
				}
				pi.CPU = delta / elapsed * 100
			}
		}
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			pi.RSS = mi.RSS
			if v, err := p.MemoryPercent(); err == nil {
				pi.MemPercent = float64(v)
			}
		}
		if st, err := p.Status(); err == nil && len(st) > 0 {
			pi.Status = st[0]
			if pi.Status == "sleeping" {
				pi.Status = "S"
			}
		}
		if n, err := p.NumThreads(); err == nil {
			pi.Threads = n
		}
		if m.opts.IncludeUser {
			if uids, err := p.Uids(); err == nil && len(uids) > 0 {
				if name, ok := m.userCache[uids[0]]; ok {
					pi.User = name
				} else if u, err := p.Username(); err == nil {
					pi.User = u
					m.userCache[uids[0]] = u
				}
			}
		}
		out = append(out, pi)
	}
	total := len(out)
	switch m.opts.SortBy {
	case "mem":
		sort.Slice(out, func(i, j int) bool {
			if out[i].RSS == out[j].RSS {
				return out[i].CPU > out[j].CPU
			}
			return out[i].RSS > out[j].RSS
		})
	default:
		sort.Slice(out, func(i, j int) bool {
			if out[i].CPU == out[j].CPU {
				return out[i].RSS > out[j].RSS
			}
			return out[i].CPU > out[j].CPU
		})
	}
	if len(out) > m.opts.TopN {
		out = out[:m.opts.TopN]
	}
	return out, total
}

// skippedFS lists pseudo filesystems that are not interesting for a disk report.
var skippedFS = map[string]bool{
	"devfs": true, "autofs": true, "procfs": true, "sysfs": true, "devpts": true,
	"fuse.gvfsd-fuse": true, "fuse.sshfs": true, "nullfs": true, "overlay": true,
	"ramfs": true, "cgroup": true, "cgroup2": true, "tracefs": true, "mqueue": true,
}

// collectDisks enumerates mounted volumes with their usage.
func collectDisks() []DiskInfo {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]DiskInfo, 0, len(parts))
	for _, p := range parts {
		if skippedFS[p.Fstype] {
			continue
		}
		if runtime.GOOS == "darwin" && strings.HasPrefix(p.Mountpoint, "/System/Volumes/") {
			// These are the same APFS container viewed through firmlinks.
			continue
		}
		if isHiddenMount(p.Mountpoint) {
			continue
		}
		if seen[p.Mountpoint] {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u == nil || u.Total == 0 {
			continue
		}
		seen[p.Mountpoint] = true
		out = append(out, DiskInfo{
			Device:     p.Device,
			Mountpoint: p.Mountpoint,
			FSType:     p.Fstype,
			Total:      u.Total,
			Used:       u.Used,
			Free:       u.Free,
			Percent:    u.UsedPercent,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out
}

// hiddenMountMarkers lists mount points that belong to the OS plumbing rather
// than to storage a user cares about.
var hiddenMountMarkers = []string{
	"/Library/Developer/CoreSimulator/Volumes",
	"/Library/Developer/CoreDevice",
	"/Volumes/Recovery",
	"/Volumes/Preboot",
	"/Volumes/VM",
	"/private/var/vm",
}

func isHiddenMount(mp string) bool {
	for _, m := range hiddenMountMarkers {
		if strings.Contains(mp, m) {
			return true
		}
	}
	return false
}
