//go:build windows

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
