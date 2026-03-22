// Package identity collects hardware/platform identifiers for agent
// self-registration with the kompakt server.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// PlatformInfo contains the discovered platform identifiers.
type PlatformInfo struct {
	Platform string
	Hostname string
	Model    string
	Vendor   string
	Metadata map[string]string
}

// Collect gathers platform info using runtime detection.
func Collect() PlatformInfo {
	hostname := getHostname()
	meta := map[string]string{
		"os":   osDescription(),
		"type": runtime.GOOS,
		"arch": runtime.GOARCH,
	}
	vendor := getVendor()
	model := getModel()
	if vendor != "" {
		meta["vendor"] = vendor
	}
	if model != "" {
		meta["model"] = model
	}

	if id := collectID(meta); id != "" {
		meta["machine_id"] = id
	}
	return PlatformInfo{
		Platform: detectPlatform(),
		Hostname: hostname,
		Model:    model,
		Vendor:   vendor,
		Metadata: meta,
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
	if isLikelyVM(meta) {
		if guid := vmGUID(); guid != "" {
			meta["vm_guid"] = guid
			return guid
		}
	}
	if serial := hardwareSerial(); serial != "" {
		meta["serial_number"] = serial
		return serial
	}
	if guid := vmGUID(); guid != "" {
		meta["vm_guid"] = guid
		return guid
	}
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

func isLikelyVM(meta map[string]string) bool {
	vendor := strings.ToLower(strings.TrimSpace(meta["vendor"]))
	model := strings.ToLower(strings.TrimSpace(meta["model"]))
	v := vendor + " " + model
	markers := []string{
		"vmware", "virtualbox", "kvm", "qemu", "xen", "hyper-v", "virtual machine",
		"microsoft corporation", "amazon ec2", "google compute", "openstack", "parallels",
	}
	for _, m := range markers {
		if strings.Contains(v, m) {
			return true
		}
	}
	return false
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(strings.Trim(line, "\""))
			if line == "" {
				continue
			}
			if strings.Contains(line, "=") {
				_, rhs, ok := strings.Cut(line, "=")
				if ok {
					line = strings.TrimSpace(rhs)
				}
			}
			if line == "" {
				continue
			}
			line = strings.Trim(line, "\";")
			if strings.Contains(strings.ToLower(line), "<class ") {
				continue
			}
			return line
		}
	}
	return ""
}

func cleanHardwareValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\x00")
	if strings.EqualFold(v, "unknown") || strings.EqualFold(v, "none") || strings.EqualFold(v, "not specified") {
		return ""
	}
	return v
}

