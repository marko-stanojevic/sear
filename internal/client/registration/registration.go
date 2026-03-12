// Package registration collects hardware/platform identifiers for client
// self-registration with the sear daemon.
package registration

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

// Collect gathers platform info.  If platformHint is "auto" (or empty) the
// platform is detected automatically; otherwise the hint is used.
func Collect(platformHint string) PlatformInfo {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	meta := map[string]string{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	}

	platform := strings.ToLower(platformHint)
	if platform == "" || platform == "auto" {
		platform = detectPlatform()
	}

	id := collectID(platform, meta)
	return PlatformInfo{
		Platform: platform,
		ID:       id,
		Hostname: hostname,
		Metadata: meta,
	}
}

// detectPlatform tries to identify the running environment.
func detectPlatform() string {
	// Azure: check IMDS.
	if fileExists("/var/lib/waagent") {
		return "azure"
	}
	// AWS: check common path.
	if fileExists("/var/lib/cloud/instance/instance-id") {
		return "aws"
	}
	// GCP: check metadata server marker.
	if fileExists("/etc/google_osconfig_agent.conf") {
		return "gcp"
	}
	return "baremetal"
}

// collectID returns the best available unique ID for the platform.
func collectID(platform string, meta map[string]string) string {
	switch platform {
	case "azure":
		if id := readFile("/sys/class/dmi/id/product_uuid"); id != "" {
			return "azure-" + id
		}
	case "aws":
		if id := readFile("/var/lib/cloud/instance/instance-id"); id != "" {
			return "aws-" + id
		}
	case "gcp":
		if id := readFile("/sys/class/dmi/id/product_serial"); id != "" {
			return "gcp-" + id
		}
	}

	// Baremetal / generic: try DMI serial number.
	if serial := readFile("/sys/class/dmi/id/product_serial"); serial != "" && serial != "Unknown" {
		meta["dmi_serial"] = serial
		return serial
	}
	// Fallback: stable MAC address.
	if mac := firstStableMAC(); mac != "" {
		return mac
	}
	// Last resort: random ID persisted by the caller.
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
		// Skip virtual/loopback.
		if strings.HasPrefix(i.Name, "lo") || strings.HasPrefix(i.Name, "veth") ||
			strings.HasPrefix(i.Name, "docker") || strings.HasPrefix(i.Name, "virbr") {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
