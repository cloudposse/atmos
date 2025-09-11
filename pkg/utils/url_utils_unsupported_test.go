//go:build \!linux && \!darwin && \!windows
// +build \!linux,\!darwin,\!windows

package utils

import (
	"errors"
	"os"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestOpenUrl_UnsupportedPlatform_ReturnsWrappedError(t *testing.T) {
	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	err := OpenUrl("https://example.com")
	if err == nil {
		t.Fatalf("expected error on unsupported platform")
	}
	if \!errors.Is(err, errUtils.ErrUnsupportedPlatform) {
		t.Fatalf("expected error to wrap ErrUnsupportedPlatform; got: %v", err)
	}
}