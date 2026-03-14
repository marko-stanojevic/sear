// Package identity collects hardware/platform identifiers for client
// self-registration with the sear daemon.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"runtime"
	"strings"
)

// PlatformInfo contains the discovered platform identifiers.
type PlatformInfo struct {
	Platform string
	ID       string
	Hostname string
	Metadata map[string]string
}

// Collect gathers platform info and resolves the configured platform hint.
func Collect(platformHint string) PlatformInfo {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	meta := map[string]string{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	}
	if desc := osDescription(); desc != "" {
		meta["os_description"] = desc
	}

	platform := normalizePlatformHint(platformHint)

	id := collectID(meta)
	return PlatformInfo{
		Platform: platform,
		ID:       id,
		Hostname: hostname,
		Metadata: meta,
	}
}

func normalizePlatformHint(hint string) string {
	v := strings.ToLower(strings.TrimSpace(hint))
	switch v {
	case "", "auto":
		return detectPlatform()
	case "linux":
		return "linux"
	case "mac":
		return "mac"
	case "windows":
		return "windows"
	default:
		return detectPlatform()
	}
}

// ── Platform detection ────────────────────────────────────────────────────────

func detectPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// ── ID collection ─────────────────────────────────────────────────────────────

func collectID(meta map[string]string) string {
	// Use DMI serial, stable MAC, or a random fallback.
	return baremetalID(meta)
}

func baremetalID(meta map[string]string) string {
	// Try DMI product serial (Linux sysfs path).
	if serial := readFile("/sys/class/dmi/id/product_serial"); serial != "" &&
		serial != "Unknown" && serial != "Not Specified" {
		meta["dmi_serial"] = serial
		return serial
	}
	// Try chassis serial.
	if serial := readFile("/sys/class/dmi/id/chassis_serial"); serial != "" &&
		serial != "Unknown" && serial != "Not Specified" {
		meta["dmi_chassis_serial"] = serial
		return serial
	}
	// Stable MAC address of the first non-virtual interface.
	if mac := firstStableMAC(); mac != "" {
		meta["mac_address"] = mac
		return mac
	}
	// Last resort: random ID. This will change on each registration if the
	// state file is missing, so it is a fallback only.
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "rnd-" + hex.EncodeToString(b)
}

func firstStableMAC() string {
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		mac := i.HardwareAddr.String()
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		// Skip virtual/loopback interfaces.
		name := i.Name
		if strings.HasPrefix(name, "lo") ||
			strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "virbr") ||
			strings.HasPrefix(name, "br-") {
			continue
		}
		return strings.ReplaceAll(mac, ":", "")
	}
	return ""
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func osDescription() string {
	if runtime.GOOS != "linux" {
		return runtime.GOOS
	}

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
