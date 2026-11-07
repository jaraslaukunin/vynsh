package sysinfo

import (
	"math"
	"testing"
	"time"
)

func TestCollectSnapshot(t *testing.T) {
	m := NewMonitor(Options{Interval: 50 * time.Millisecond, TopN: 5, SortBy: "cpu", IncludeProcs: true})
	snap, err := m.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if snap.Hostname == "" {
		t.Error("hostname is empty")
	}
	if snap.CPU.Cores <= 0 {
		t.Errorf("cores = %d, want > 0", snap.CPU.Cores)
	}
	if snap.Mem.Total == 0 {
		t.Error("memory total is 0")
	}
	if snap.Mem.Used > snap.Mem.Total {
		t.Errorf("used (%d) exceeds total (%d)", snap.Mem.Used, snap.Mem.Total)
	}
	if snap.Mem.Percent < 0 || snap.Mem.Percent > 100 {
		t.Errorf("memory percent out of range: %v", snap.Mem.Percent)
	}
	if math.IsNaN(snap.CPU.Percent) || snap.CPU.Percent < 0 {
		t.Errorf("cpu percent = %v", snap.CPU.Percent)
	}
	if len(snap.Disks) == 0 {
		t.Error("no disks reported")
	}
	for _, d := range snap.Disks {
		if d.Total == 0 {
			t.Errorf("disk %s has total 0", d.Mountpoint)
		}
		if d.Used > d.Total {
			t.Errorf("disk %s used %d > total %d", d.Mountpoint, d.Used, d.Total)
		}
	}
	if snap.ProcCount == 0 || len(snap.Procs) == 0 {
		t.Error("no processes reported")
	}
	if len(snap.Procs) > 5 {
		t.Errorf("procs = %d, want at most TopN", len(snap.Procs))
	}
	for _, p := range snap.Procs {
		if p.PID == 0 {
			t.Error("process without pid")
		}
		if p.CPU < 0 || math.IsNaN(p.CPU) {
			t.Errorf("process %s cpu = %v", p.Name, p.CPU)
		}
	}
}

func TestSortByMemory(t *testing.T) {
	m := NewMonitor(Options{Interval: 30 * time.Millisecond, TopN: 3, SortBy: "mem", IncludeProcs: true})
	snap, err := m.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for i := 1; i < len(snap.Procs); i++ {
		if snap.Procs[i-1].RSS < snap.Procs[i].RSS {
			t.Fatalf("processes are not sorted by memory: %+v", snap.Procs)
		}
	}
}

func TestCollectWithoutProcesses(t *testing.T) {
	m := NewMonitor(Options{Interval: 20 * time.Millisecond, IncludeProcs: false})
	snap, err := m.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(snap.Procs) != 0 || snap.ProcCount != 0 {
		t.Errorf("processes should be skipped, got %d", len(snap.Procs))
	}
	if snap.Mem.Total == 0 {
		t.Error("memory info should still be collected")
	}
}

func TestMonitorReusesState(t *testing.T) {
	m := NewMonitor(Options{Interval: 20 * time.Millisecond, TopN: 2, IncludeProcs: false})
	first, err := m.Collect()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if first.Mem.Total != second.Mem.Total {
		t.Errorf("total memory changed between samples: %d vs %d", first.Mem.Total, second.Mem.Total)
	}
	if !second.Time.After(first.Time) {
		t.Error("second snapshot must be newer")
	}
}
