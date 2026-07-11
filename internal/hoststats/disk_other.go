//go:build !linux

package hoststats

// readDisk is a no-op off Linux (dev machines); the strip hides the meter.
func readDisk(string) (used, total uint64, ok bool) {
	return 0, 0, false
}
