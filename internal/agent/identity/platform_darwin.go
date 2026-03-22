//go:build darwin

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
	hostname = strings.TrimSuffix(hostname, ".local")
	return hostname
}


func getVendor() string {
	return "Apple"
}
