package hoststats

import (
	"runtime"
	"testing"
)

func TestReadMem(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc/meminfo is Linux-only")
	}
	used, total, ok := readMem()
	if !ok {
		t.Fatal("expected mem read to succeed on linux")
	}
	if total == 0 || used > total {
		t.Fatalf("implausible mem: used=%d total=%d", used, total)
	}
}

func TestSampleCPU(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc/stat is Linux-only")
	}
	s := NewSampler(0)
	if _, ok := s.sampleCPU(); !ok {
		t.Fatal("expected first cpu sample to read ok")
	}
	pct, ok := s.sampleCPU()
	if !ok {
		t.Fatal("expected second cpu sample ok")
	}
	if pct < 0 || pct > 100 {
		t.Fatalf("cpu%% out of range: %f", pct)
	}
}

func TestSnapshotOffLinuxIsUnavailable(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this asserts graceful degradation off-linux")
	}
	s := NewSampler(0)
	s.update()
	if s.Snapshot().Available {
		t.Fatal("expected Available=false without /proc")
	}
}
