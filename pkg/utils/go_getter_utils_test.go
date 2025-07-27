package utils

import (
	"strings"
	"testing"

	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
)

var originalDetectors = getter.Detectors

func TestValidateURI(t *testing.T) {
	if err := ValidateURI(""); err == nil {
		t.Error("Expected error for empty URI, got nil")
	}
	uri := strings.Repeat("a", 2050)
	if err := ValidateURI(uri); err == nil {
		t.Error("Expected error for too-long URI, got nil")
	}
	if err := ValidateURI("http://example.com/../secret"); err == nil {
		t.Error("Expected error for path traversal sequence, got nil")
	}
	if err := ValidateURI("http://example.com/space test"); err == nil {
		t.Error("Expected error for spaces in URI, got nil")
	}
	if err := ValidateURI("http://example.com/path"); err != nil {
		t.Errorf("Expected valid URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo/path"); err != nil {
		t.Errorf("Expected valid OCI URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo"); err == nil {
		t.Error("Expected error for invalid OCI URI format, got nil")
	}
}

func TestIsValidScheme(t *testing.T) {
	valid := []string{"http", "https", "git", "ssh", "git::https", "git::ssh"}
	for _, scheme := range valid {
		if !IsValidScheme(scheme) {
			t.Errorf("Expected scheme %s to be valid", scheme)
		}
	}
	if IsValidScheme("ftp") {
		t.Error("Expected scheme ftp to be invalid")
	}
}

func TestValidateURI_ErrorPaths(t *testing.T) {
	err := ValidateURI("http://example.com/with space")
	if err == nil {
		t.Error("Expected error for URI with space")
	}
	err = ValidateURI("http://example.com/../secret")
	if err == nil {
		t.Error("Expected error for URI with path traversal")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	getter.Detectors = originalDetectors
	errUtils.Exit(code)
}
