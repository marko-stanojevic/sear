//go:build linux || darwin

package iso

import "syscall"

// availableDiskBytes returns the number of free bytes available to the
// current user on the filesystem containing path.
func availableDiskBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
