//go:build windows

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
		runAndTrim("wmic", "computersystem", "get", "model", "/value"),
		runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).Model"),
	)
}

func getVendor() string {
	return firstNonEmpty(
		runAndTrim("wmic", "computersystem", "get", "manufacturer", "/value"),
		runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).Manufacturer"),
	)
}

func hardwareSerial() string {
	return cleanHardwareValue(firstNonEmpty(
		runAndTrim("wmic", "bios", "get", "serialnumber", "/value"),
		runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_BIOS).SerialNumber"),
	))
}

func vmGUID() string {
	return cleanHardwareValue(firstNonEmpty(
		runAndTrim("wmic", "csproduct", "get", "uuid", "/value"),
		runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystemProduct).UUID"),
	))
}

func osDescription() string {
	caption := firstNonEmpty(
		runAndTrim("wmic", "os", "get", "caption", "/value"),
		runAndTrim("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_OperatingSystem).Caption"),
	)
	caption = strings.TrimPrefix(caption, "Microsoft ")
	if caption != "" {
		return caption
	}
	return "windows"
}
