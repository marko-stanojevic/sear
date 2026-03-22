//go:build linux

package identity

import (
	"os"
	"strings"
)

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

func hardwareSerial() string {
	if serial := cleanHardwareValue(readFile("/sys/class/dmi/id/product_serial")); serial != "" {
		return serial
	}
	return cleanHardwareValue(readFile("/sys/class/dmi/id/chassis_serial"))
}

func vmGUID() string {
	return cleanHardwareValue(firstNonEmpty(
		readFile("/sys/class/dmi/id/product_uuid"),
		readFile("/sys/hypervisor/uuid"),
	))
}

func osDescription() string {
	vals := parseOSRelease("/etc/os-release")
	if pretty := vals["PRETTY_NAME"]; pretty != "" {
		return pretty
	}
	name := vals["NAME"]
	version := vals["VERSION"]
	if name != "" && version != "" {
		return strings.TrimSpace(name + " " + version)
	}
	if name != "" {
		return name
	}
	return "linux"
}

func parseOSRelease(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[k] = strings.Trim(v, "\"'")
	}
	return vals
}
