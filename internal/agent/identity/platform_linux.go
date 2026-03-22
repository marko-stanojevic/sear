//go:build linux

package identity

import "os"

func getHostname() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return hostname
}

func getModel() string {
	return firstNonEmpty(
		readFile("/sys/class/dmi/id/product_name"),
		readFile("/sys/devices/virtual/dmi/id/product_name"),
		readFile("/proc/device-tree/model"),
	)
}

func getVendor() string {
	return firstNonEmpty(
		readFile("/sys/class/dmi/id/sys_vendor"),
		readFile("/sys/devices/virtual/dmi/id/sys_vendor"),
	)
}
