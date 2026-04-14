//go:build !linux && !darwin

package iso

// availableDiskBytes is not supported on this platform and always returns 0.
func availableDiskBytes(_ string) (uint64, error) {
	return 0, nil
}
