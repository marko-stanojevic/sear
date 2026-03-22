// Package identity collects hardware/platform identifiers for agent
// self-registration with the kompakt server.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

func hardwareSerial() string {
	switch runtime.GOOS {
	case "linux":
		if serial := cleanHardwareValue(readFile("/sys/class/dmi/id/product_serial")); serial != "" {
			return serial
		}
		if serial := cleanHardwareValue(readFile("/sys/class/dmi/id/chassis_serial")); serial != "" {
			return serial
		}
	case "darwin":
		return cleanHardwareValue(ioregValue("IOPlatformSerialNumber"))
	case "windows":
		return cleanHardwareValue(firstNonEmpty(
			runAndTrim("wmic", "bios", "get", "serialnumber", "/value"),
			runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_BIOS).SerialNumber"),
		))
	}
	return ""
}

func vmGUID() string {
	switch runtime.GOOS {
	case "linux":
		return cleanHardwareValue(firstNonEmpty(
			readFile("/sys/class/dmi/id/product_uuid"),
			readFile("/sys/hypervisor/uuid"),
		))
	case "darwin":
		return cleanHardwareValue(ioregValue("IOPlatformUUID"))
	case "windows":
		return cleanHardwareValue(firstNonEmpty(
			runAndTrim("wmic", "csproduct", "get", "uuid", "/value"),
			runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystemProduct).UUID"),
		))
	}
	return ""
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
	switch runtime.GOOS {
	case "windows":
		caption := firstNonEmpty(
			runAndTrim("wmic", "os", "get", "caption", "/value"),
			runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_OperatingSystem).Caption"),
		)
		caption = strings.TrimPrefix(caption, "Microsoft ")
		if caption != "" {
			return caption
		}
		return "windows"
	case "darwin":
		name := runAndTrim("sw_vers", "-productName")
		version := runAndTrim("sw_vers", "-productVersion")
		if name != "" && version != "" {
			return name + " " + version
		}
		if name != "" {
			return name
		}
		return "macOS"
	case "linux":
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
	default:
		return runtime.GOOS
	}
}

func runAndTrim(cmd string, args ...string) string {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

func ioregValue(key string) string {
	out := runAndTrim("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	if out == "" {
		return ""
	}
	needle := strings.ToLower(key)
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(l), needle) {
			continue
		}
		_, rhs, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		return strings.Trim(strings.TrimSpace(rhs), "\";")
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
