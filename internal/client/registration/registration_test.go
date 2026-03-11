package registration_test

import (
	"testing"

	"github.com/marko-stanojevic/sear/internal/client/registration"
)

func TestCollect_Baremetal(t *testing.T) {
	info := registration.Collect("baremetal")
	if info.Platform != "baremetal" {
		t.Errorf("Platform = %q; want baremetal", info.Platform)
	}
	if info.Hostname == "" {
		t.Error("Hostname is empty")
	}
	if info.ID == "" {
		t.Error("ID is empty")
	}
	if info.Metadata == nil {
		t.Error("Metadata is nil")
	}
	if info.Metadata["os"] == "" {
		t.Error("Metadata[os] is empty")
	}
}

func TestCollect_Auto(t *testing.T) {
	info := registration.Collect("auto")
	if info.Platform == "" {
		t.Error("Platform is empty")
	}
	if info.ID == "" {
		t.Error("ID is empty")
	}
}

func TestCollect_EmptyHint(t *testing.T) {
	info := registration.Collect("")
	if info.Platform == "" {
		t.Error("Platform should default when hint is empty")
	}
}
