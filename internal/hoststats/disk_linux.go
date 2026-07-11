//go:build linux

package hoststats

import "syscall"

// readDisk reports used/total bytes of the filesystem containing path. On a
// bind-mounted stacks dir this is the host's disk; falls back to "/" when the
// configured path doesn't resolve (e.g. dev without the mount).
func readDisk(path string) (used, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		if path == "/" {
			return 0, 0, false
		}
		if err := syscall.Statfs("/", &st); err != nil {
			return 0, 0, false
		}
	}
	if st.Blocks == 0 {
		return 0, 0, false
	}
	bs := uint64(st.Bsize)
	total = st.Blocks * bs
	free := st.Bavail * bs // space available to non-root, matching df's "Avail"
	if free > total {
		free = total
	}
	return total - free, total, true
}
