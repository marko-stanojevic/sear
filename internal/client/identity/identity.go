// Package identity collects hardware/platform identifiers for client
// self-registration with the sear daemon.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// PlatformInfo contains the discovered platform identifiers.
type PlatformInfo struct {
	Platform string
	ID       string
	Hostname string
	Metadata map[string]string
}

var imdsClient = &http.Client{Timeout: 2 * time.Second}

// Collect gathers platform info. If platformHint is "auto" or empty the
// platform is auto-detected; otherwise the hint is used directly.
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

// ── Platform detection ────────────────────────────────────────────────────────

func detectPlatform() string {
	// AWS: IMDSv2 token endpoint responds in ~1 ms on EC2.
	if isAWS() {
		return "aws"
	}
	// Azure: IMDS endpoint is present on all Azure VMs.
	if isAzure() {
		return "azure"
	}
	// GCP: metadata server returns a recognisable header.
	if isGCP() {
		return "gcp"
	}
	return "baremetal"
}

func isAWS() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	// Step 1: get IMDSv2 token.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		"http://169.254.169.254/latest/api/token", nil)
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "10")
	resp, err := imdsClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func isAzure() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://169.254.169.254/metadata/instance?api-version=2021-02-01", nil)
	req.Header.Set("Metadata", "true")
	resp, err := imdsClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func isGCP() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/instance/id", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := imdsClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ── ID collection ─────────────────────────────────────────────────────────────

func collectID(platform string, meta map[string]string) string {
	switch platform {
	case "aws":
		if id := awsInstanceID(meta); id != "" {
			return id
		}
	case "azure":
		if id := azureVMID(meta); id != "" {
			return id
		}
	case "gcp":
		if id := gcpInstanceID(meta); id != "" {
			return id
		}
	}
	// Baremetal / generic: try DMI serial, then stable MAC, then random.
	return baremetalID(meta)
}

func awsInstanceID(meta map[string]string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Get IMDSv2 token first.
	tokenReq, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		"http://169.254.169.254/latest/api/token", nil)
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "30")
	tokenResp, err := imdsClient.Do(tokenReq)
	if err != nil {
		return ""
	}
	tokenBytes, _ := io.ReadAll(tokenResp.Body)
	tokenResp.Body.Close()
	token := strings.TrimSpace(string(tokenBytes))

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://169.254.169.254/latest/meta-data/instance-id", nil)
	req.Header.Set("X-aws-ec2-metadata-token", token)
	resp, err := imdsClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	id := strings.TrimSpace(string(b))
	if id != "" {
		meta["aws_instance_id"] = id
		// Also grab region.
		if region := awsMeta(token, "placement/region"); region != "" {
			meta["aws_region"] = region
		}
	}
	return "aws-" + id
}

func awsMeta(token, path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://169.254.169.254/latest/meta-data/"+path, nil)
	req.Header.Set("X-aws-ec2-metadata-token", token)
	resp, err := imdsClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(b))
}

func azureVMID(meta map[string]string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://169.254.169.254/metadata/instance?api-version=2021-02-01", nil)
	req.Header.Set("Metadata", "true")
	resp, err := imdsClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var payload struct {
		Compute struct {
			VMID           string `json:"vmId"`
			ResourceGroup  string `json:"resourceGroupName"`
			Location       string `json:"location"`
		} `json:"compute"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	if payload.Compute.VMID != "" {
		meta["azure_vm_id"] = payload.Compute.VMID
		meta["azure_resource_group"] = payload.Compute.ResourceGroup
		meta["azure_location"] = payload.Compute.Location
	}
	return "azure-" + payload.Compute.VMID
}

func gcpInstanceID(meta map[string]string) string {
	id := gcpMeta("instance/id")
	if id == "" {
		return ""
	}
	meta["gcp_instance_id"] = id
	if zone := gcpMeta("instance/zone"); zone != "" {
		// zone is "projects/PROJECT/zones/ZONE" — extract last segment.
		parts := strings.Split(zone, "/")
		meta["gcp_zone"] = parts[len(parts)-1]
	}
	return "gcp-" + id
}

func gcpMeta(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/%s", path), nil)
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := imdsClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(b))
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
