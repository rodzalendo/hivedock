// Package hoststats samples CPU and memory from /proc. Inside a container these
// reflect the container's cgroup-limited view, not the physical host — that's
// expected and documented (see docs/DEPLOYMENT.md); the strip shows what the
// container can see. Off-Linux (dev on Windows/macOS) it reports Available=false.
package hoststats

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Snapshot is the latest sampled host state.
type Snapshot struct {
	Available     bool    `json:"available"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemUsedBytes  uint64  `json:"memUsedBytes"`
	MemTotalBytes uint64  `json:"memTotalBytes"`
	NumCPU        int     `json:"numCpu"`
	SampledAt     string  `json:"sampledAt,omitempty"`
}

// Sampler periodically refreshes a Snapshot in the background so the HTTP
// handler can return instantly (no per-request sampling latency).
type Sampler struct {
	interval time.Duration

	mu      sync.RWMutex
	current Snapshot

	prevIdle  uint64
	prevTotal uint64
}

// NewSampler creates a sampler with the given refresh interval.
func NewSampler(interval time.Duration) *Sampler {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Sampler{interval: interval}
}

// Snapshot returns the most recent sample.
func (s *Sampler) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Run samples until ctx is cancelled. Safe to call in a goroutine.
func (s *Sampler) Run(ctx context.Context) {
	// Prime CPU deltas so the first published sample is meaningful.
	s.sampleCPU()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.update()
		}
	}
}

func (s *Sampler) update() {
	cpu, cpuOK := s.sampleCPU()
	used, total, memOK := readMem()

	snap := Snapshot{
		Available: cpuOK || memOK,
		NumCPU:    countCPUs(),
		SampledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if cpuOK {
		snap.CPUPercent = cpu
	}
	if memOK {
		snap.MemUsedBytes = used
		snap.MemTotalBytes = total
	}

	s.mu.Lock()
	s.current = snap
	s.mu.Unlock()
}

// sampleCPU reads /proc/stat and returns CPU% since the previous call.
func (s *Sampler) sampleCPU() (float64, bool) {
	idle, total, ok := readCPU()
	if !ok {
		return 0, false
	}
	var pct float64
	if s.prevTotal != 0 && total > s.prevTotal {
		idleDelta := float64(idle - s.prevIdle)
		totalDelta := float64(total - s.prevTotal)
		pct = (1 - idleDelta/totalDelta) * 100
		if pct < 0 {
			pct = 0
		}
	}
	s.prevIdle = idle
	s.prevTotal = total
	return pct, true
}

// readCPU parses the aggregate "cpu" line of /proc/stat into (idle, total).
func readCPU() (idle, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i, v := range fields[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			continue
		}
		total += n
		// idle is field index 3 (idle) + 4 (iowait) in the cpu line.
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return idle, total, true
}

// readMem parses /proc/meminfo into (usedBytes, totalBytes).
func readMem() (used, total uint64, ok bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, found := strings.Cut(sc.Text(), ":")
		if !found {
			continue
		}
		kb, err := strconv.ParseUint(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), "kB")), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			memTotal, haveTotal = kb*1024, true
		case "MemAvailable":
			memAvailable, haveAvail = kb*1024, true
		}
	}
	if !haveTotal || !haveAvail || memTotal == 0 {
		return 0, 0, false
	}
	if memAvailable > memTotal {
		memAvailable = memTotal
	}
	return memTotal - memAvailable, memTotal, true
}

func countCPUs() int {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			n++
		}
	}
	return n
}
