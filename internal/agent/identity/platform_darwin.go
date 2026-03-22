//go:build darwin

package identity

import (
	"log/slog"
	"os"
	"strings"

	"howett.net/plist"
)

func getHostname() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	hostname = strings.TrimSuffix(hostname, ".local")
	return hostname
}

func getVendor() string {
	return "Apple"
}

// getModel returns the marketing name for the Mac model, if available.
func getModel() string {
	model := firstNonEmpty(runAndTrim("sysctl", "-n", "hw.model"))
	if model == "" {
		slog.Debug("getModel: sysctl hw.model returned empty")
		return "unknown"
	}
	plistPath := "/System/Library/PrivateFrameworks/ServerInformation.framework/SIMachineAttributes.plist"
	f, err := os.Open(plistPath)
	if err != nil {
		slog.Debug("getModel: could not open plist", "path", plistPath, "error", err)
		return model // fallback to identifier
	}
	defer f.Close()
	var data map[string]interface{}
	decoder := plist.NewDecoder(f)
	if err := decoder.Decode(&data); err != nil {
		slog.Debug("getModel: plist decode failed", "error", err)
		return model
	}
	entry, ok := data[model].(map[string]interface{})
	if !ok {
		slog.Debug("getModel: model entry not found in plist", "model", model)
		return model
	}
	loc, ok := entry["_LOCALIZABLE_"].(map[string]interface{})
	if !ok {
		slog.Debug("getModel: _LOCALIZABLE_ not found in plist entry", "model", model)
		return model
	}
	marketing, ok := loc["marketingModel"].(string)
	if ok && marketing != "" {
		slog.Debug("getModel: found marketing model", "marketingModel", marketing)
		return marketing
	}
	slog.Debug("getModel: marketingModel not found or empty", "model", model)
	return model
}

func hardwareSerial() string {
	return cleanHardwareValue(ioregValue("IOPlatformSerialNumber"))
}

func vmGUID() string {
	return cleanHardwareValue(ioregValue("IOPlatformUUID"))
}

func osDescription() string {
	name := runAndTrim("sw_vers", "-productName")
	version := runAndTrim("sw_vers", "-productVersion")
	if name != "" && version != "" {
		return name + " " + version
	}
	if name != "" {
		return name
	}
	return "macOS"
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
