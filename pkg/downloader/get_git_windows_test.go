//go:build windows
// +build windows

package downloader

import (
	"context"
	"testing"
)

// TestCheckGitVersion_WindowsSuffix_Stripped verifies Windows-specific git version string handling.
func TestCheckGitVersion_WindowsSuffix_Stripped(t *testing.T) {
	// On Windows, version string may contain ".windows." and should be stripped.
	writeFakeGit(t, "git version 2.20.1.windows.1", 0)

	// With suffix removed, this compares as 2.20.1
	err := checkGitVersion(context.Background(), "2.18.0")
	if err != nil {
		t.Fatalf("expected no error after stripping .windows. suffix, got %v", err)
	}

	err = checkGitVersion(context.Background(), "2.30.0")
	if err == nil {
		t.Fatalf("expected error: 2.20.1 < 2.30.0")
	}
}
