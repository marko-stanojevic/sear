// Package identity collects hardware/platform identifiers for agent
// self-registration with the kompakt server.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"log"
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

// Collect gathers platform info and resolves the configured platform hint.
func Collect(platformHint string) PlatformInfo {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	meta := map[string]string{
		"os":   osDescription(),
		"type": runtime.GOOS,
		"arch": runtime.GOARCH,
	}
	vendor, model := hardwareInfo()
	if vendor != "" {
		meta["vendor"] = vendor
	}
	if model != "" {
		meta["model"] = model
	}

	platform := normalizePlatformHint(platformHint)
	if id := collectID(meta); id != "" {
		meta["machine_id"] = id
	}
	return PlatformInfo{
		Platform: platform,
		Hostname: hostname,
		Model:    model,
		Vendor:   vendor,
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

func hardwareInfo() (vendor string, model string) {
	switch runtime.GOOS {
	case "linux":
		vendor = firstNonEmpty(
			readFile("/sys/class/dmi/id/sys_vendor"),
			readFile("/sys/devices/virtual/dmi/id/sys_vendor"),
		)
		model = firstNonEmpty(
			readFile("/sys/class/dmi/id/product_name"),
			readFile("/sys/devices/virtual/dmi/id/product_name"),
			readFile("/proc/device-tree/model"),
		)
	case "darwin":
		vendor = "Apple"
		model = firstNonEmpty(runAndTrim("sysctl", "-n", "hw.model"))
	case "windows":
		vendor = firstNonEmpty(
			runAndTrim("wmic", "computersystem", "get", "manufacturer", "/value"),
			runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).Manufacturer"),
		)
		model = firstNonEmpty(
			runAndTrim("wmic", "computersystem", "get", "model", "/value"),
			runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).Model"),
		)
	}

	vendor = cleanHardwareValue(vendor)
	model = cleanHardwareValue(model)

	if runtime.GOOS == "linux" && vendor == "" && model == "" {
		if cv, cm := detectContainer(); cv != "" {
			vendor, model = cv, cm
			log.Printf("hardware: DMI unavailable, detected container runtime: %s", cv)
		}
	}

	return vendor, model
}

// detectContainer returns the container runtime name and a generic model label
// when the process is running inside a container (Docker, Podman, LXC, etc.).
func detectContainer() (vendor, model string) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "Docker", "container"
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return "Podman", "container"
	}
	cgroup := readFile("/proc/1/cgroup")
	lower := strings.ToLower(cgroup)
	switch {
	case strings.Contains(lower, "docker"):
		return "Docker", "container"
	case strings.Contains(lower, "podman"):
		return "Podman", "container"
	case strings.Contains(lower, "lxc"):
		return "LXC", "container"
	case strings.Contains(lower, "containerd"):
		return "containerd", "container"
	}
	return "", ""
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
