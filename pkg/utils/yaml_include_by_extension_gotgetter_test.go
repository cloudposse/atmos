package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestShouldFetchRemoteComprehensive provides comprehensive test coverage.
// For all go-getter supported URL formats to prevent regressions.
func TestShouldFetchRemoteComprehensive(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
		desc     string
	}{
		// GitHub formats.
		{"github.com/hashicorp/terraform", true, "GitHub shorthand without protocol"},
		{"github.com/hashicorp/terraform.git", true, "GitHub shorthand with .git"},
		{"github.com/hashicorp/terraform?ref=v1.0.0", true, "GitHub shorthand with ref"},
		{"github.com/hashicorp/terraform//subdir", true, "GitHub shorthand with subdir"},
		{"https://github.com/hashicorp/terraform", true, "GitHub HTTPS URL"},
		{"https://github.com/hashicorp/terraform.git", true, "GitHub HTTPS with .git"},
		{"git::https://github.com/hashicorp/terraform.git", true, "Git protocol wrapper"},
		{"git::ssh://git@github.com/hashicorp/terraform.git", true, "Git SSH protocol"},
		{"git@github.com:hashicorp/terraform.git", true, "Git SSH format"},

		// GitLab formats.
		{"gitlab.com/myorg/myrepo", true, "GitLab shorthand"},
		{"https://gitlab.com/myorg/myrepo", true, "GitLab HTTPS"},
		{"git::https://gitlab.com/myorg/myrepo.git", true, "GitLab with git protocol"},
		{"git@gitlab.com:myorg/myrepo.git", true, "GitLab SSH format"},
		{"git::ssh://git@gitlab.com/myorg/myrepo.git", true, "GitLab SSH protocol"},

		// Bitbucket formats.
		{"https://bitbucket.org/myorg/myrepo", true, "Bitbucket HTTPS"},
		{"git::https://bitbucket.org/myorg/myrepo.git", true, "Bitbucket with git protocol"},
		{"git@bitbucket.org:myorg/myrepo.git", true, "Bitbucket SSH format"},
		{"git::ssh://git@bitbucket.org/myorg/myrepo.git", true, "Bitbucket SSH protocol"},
		{"bitbucket.org/myorg/myrepo", false, "Bitbucket shorthand (not supported without protocol)"},

		// S3 formats.
		{"s3::https://s3.amazonaws.com/bucket/key", true, "S3 HTTPS"},
		{"s3::https://s3-eu-west-1.amazonaws.com/bucket/key", true, "S3 regional endpoint"},
		{"s3::s3://bucket/key", true, "S3 protocol"},

		// GCS formats.
		{"gcs::https://storage.googleapis.com/bucket/path", true, "GCS HTTPS"},
		{"gcs::https://bucket.storage.googleapis.com/path", true, "GCS bucket URL"},

		// HTTP/HTTPS.
		{"http://example.com/file.yaml", true, "HTTP URL"},
		{"https://example.com/file.yaml", true, "HTTPS URL"},
		{"https://raw.githubusercontent.com/org/repo/main/file.yaml", true, "Raw GitHub content"},

		// Mercurial.
		{"hg::https://example.com/hg-repo", true, "Mercurial HTTPS"},

		// File URLs (should be local).
		{"file:///absolute/path", false, "File protocol absolute"},
		{"file://./relative/path", false, "File protocol relative"},

		// Local paths.
		{"./relative/path.yaml", false, "Relative path with ./"},
		{"../parent/path.yaml", false, "Parent directory path"},
		{"relative/path.yaml", false, "Simple relative path"},
		{"/absolute/path.yaml", false, "Absolute path"},
		{"~/home/path.yaml", false, "Home directory path"},
		{"path/to/file.yaml", false, "Multi-level relative path"},
		{"file.yaml", false, "Simple filename"},
		{"stacks/objects/ips.yaml", false, "Stack relative path"},

		// Edge cases.
		{"", false, "Empty string"},
		{".", false, "Current directory"},
		{"..", false, "Parent directory"},
		{"/", false, "Root directory"},

		// Common mistakes that should still be local.
		{"github-com/org/repo", false, "Typo with dash instead of dot"},
		{"github/org/repo", false, "Missing .com"},
		{"com.github/org/repo", false, "Reversed domain"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := shouldFetchRemote(tc.path)
			assert.Equal(t, tc.expected, result,
				"shouldFetchRemote(%q) returned %v, expected %v",
				tc.path, result, tc.expected)
		})
	}
}

// TestShouldFetchRemoteWithGoGetterModifiers tests go-getter URL modifiers.
func TestShouldFetchRemoteWithGoGetterModifiers(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Double-slash for subdirectories.
		{"github.com/hashicorp/terraform//modules/vpc", true, "GitHub with subdir"},
		{"https://github.com/hashicorp/terraform//modules/vpc", true, "HTTPS with subdir"},
		{"git::https://github.com/hashicorp/terraform.git//modules/vpc", true, "Git protocol with subdir"},

		// Query parameters.
		{"github.com/hashicorp/terraform?ref=v1.0.0", true, "GitHub with ref query"},
		{"github.com/hashicorp/terraform?ref=v1.0.0&depth=1", true, "GitHub with multiple queries"},
		{"https://github.com/hashicorp/terraform?archive=tar.gz", true, "HTTPS with archive query"},

		// Combined modifiers.
		{"github.com/hashicorp/terraform//modules/vpc?ref=v1.0.0", true, "GitHub with subdir and ref"},
		{"git::https://github.com/hashicorp/terraform.git//modules/vpc?ref=main", true, "Git protocol with all modifiers"},

		// Local paths with similar patterns (should NOT be remote).
		{"path//with//double//slashes", false, "Local path with double slashes"},
		{"local/path?not=query", false, "Local path with question mark"},
		{"./local//path?ref=fake", false, "Relative path with modifiers"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := shouldFetchRemote(tc.path)
			assert.Equal(t, tc.expected, result,
				"shouldFetchRemote(%q) returned %v, expected %v",
				tc.path, result, tc.expected)
		})
	}
}
