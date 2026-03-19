package store

import (
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// ── migrateLegacyAgentFields ─────────────────────────────────────────────────

func TestMigrateLegacyAgentFields_Nil(t *testing.T) {
	// Must not panic on nil input.
	migrateLegacyAgentFields(nil)
}

func TestMigrateLegacyAgentFields_NilMetadata(t *testing.T) {
	a := &common.Agent{OS: ""}
	migrateLegacyAgentFields(a) // no-op: Metadata is nil
	if a.OS != "" {
		t.Errorf("OS should remain empty when Metadata is nil, got %q", a.OS)
	}
}

func TestMigrateLegacyAgentFields_OSFromDescription(t *testing.T) {
	a := &common.Agent{
		Metadata: map[string]string{"os_description": "Ubuntu 22.04"},
	}
	migrateLegacyAgentFields(a)
	if a.OS != "Ubuntu 22.04" {
		t.Errorf("OS = %q; want Ubuntu 22.04", a.OS)
	}
}

func TestMigrateLegacyAgentFields_OSFromOSKey(t *testing.T) {
	a := &common.Agent{
		Metadata: map[string]string{"os": "Debian 12"},
	}
	migrateLegacyAgentFields(a)
	if a.OS != "Debian 12" {
		t.Errorf("OS = %q; want Debian 12", a.OS)
	}
}

func TestMigrateLegacyAgentFields_DoesNotOverwriteExistingOS(t *testing.T) {
	a := &common.Agent{
		OS:       "AlreadySet",
		Metadata: map[string]string{"os": "ShouldBeIgnored"},
	}
	migrateLegacyAgentFields(a)
	if a.OS != "AlreadySet" {
		t.Errorf("OS = %q; want AlreadySet (must not overwrite)", a.OS)
	}
}

func TestMigrateLegacyAgentFields_VendorAndModel(t *testing.T) {
	a := &common.Agent{
		Metadata: map[string]string{"vendor": "Dell", "model": "PowerEdge R650"},
	}
	migrateLegacyAgentFields(a)
	if a.Vendor != "Dell" {
		t.Errorf("Vendor = %q; want Dell", a.Vendor)
	}
	if a.Model != "PowerEdge R650" {
		t.Errorf("Model = %q; want PowerEdge R650", a.Model)
	}
}

// ── normalizePlatform ─────────────────────────────────────────────────────────

func TestNormalizePlatform(t *testing.T) {
	tests := []struct {
		platform common.PlatformType
		osName   string
		metadata map[string]string
		want     common.PlatformType
	}{
		{platform: "linux", want: common.PlatformLinux},
		{platform: "mac", want: common.PlatformMac},
		{platform: "windows", want: common.PlatformWindows},
		{platform: "", osName: "windows", want: common.PlatformWindows},
		{platform: "auto", osName: "darwin", want: common.PlatformMac},
		{platform: "unknown_value", osName: "linux", want: common.PlatformLinux},
		{
			platform: "",
			metadata: map[string]string{"type": "windows"},
			want:     common.PlatformWindows,
		},
	}

	for _, tt := range tests {
		got := normalizePlatform(tt.platform, tt.osName, tt.metadata)
		if got != tt.want {
			t.Errorf("normalizePlatform(%q, %q, %v) = %q; want %q",
				tt.platform, tt.osName, tt.metadata, got, tt.want)
		}
	}
}

// ── platformFromOS ────────────────────────────────────────────────────────────

func TestPlatformFromOS(t *testing.T) {
	tests := []struct {
		osName   string
		metadata map[string]string
		want     common.PlatformType
	}{
		{"darwin", nil, common.PlatformMac},
		{"windows", nil, common.PlatformWindows},
		{"linux", nil, common.PlatformLinux},
		{"ubuntu", nil, common.PlatformLinux},
		{"", map[string]string{"os": "darwin"}, common.PlatformMac},
		{"", map[string]string{"type": "windows"}, common.PlatformWindows},
		{"", map[string]string{"os_type": "linux"}, common.PlatformLinux},
		{"", nil, common.PlatformLinux},
	}

	for _, tt := range tests {
		got := platformFromOS(tt.osName, tt.metadata)
		if got != tt.want {
			t.Errorf("platformFromOS(%q, %v) = %q; want %q",
				tt.osName, tt.metadata, got, tt.want)
		}
	}
}
