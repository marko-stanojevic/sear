package common

import (
	"regexp"
	"testing"
)

var ulidPattern = regexp.MustCompile(`^[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{26}$`)

func TestNewID_Length(t *testing.T) {
	id := NewID()
	if len(id) != 26 {
		t.Fatalf("NewID length = %d; want 26", len(id))
	}
}

func TestNewID_ValidCharset(t *testing.T) {
	id := NewID()
	if !ulidPattern.MatchString(id) {
		t.Fatalf("NewID %q does not match Crockford base32 pattern", id)
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := range 100 {
		id := NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewID produced duplicate at iteration %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
