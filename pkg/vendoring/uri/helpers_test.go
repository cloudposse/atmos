package uri

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasLocalPathPrefix(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Absolute paths.
		{
			name:     "absolute unix path",
			uri:      "/absolute/path/to/components",
			expected: true,
		},
		{
			name:     "absolute windows path",
			uri:      "C:\\Users\\components",
			expected: false, // Not a Unix absolute path.
		},
		// Relative paths.
		{
			name:     "current directory prefix",
			uri:      "./relative/path",
			expected: true,
		},
		{
			name:     "parent directory prefix",
			uri:      "../parent/path",
			expected: true,
		},
		// Non-local paths.
		{
			name:     "github URL",
			uri:      "github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "https URL",
			uri:      "https://github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "relative path without prefix",
			uri:      "components/terraform/vpc",
			expected: false,
		},
		// Edge cases.
		{
			name:     "single dot",
			uri:      ".",
			expected: false, // Only matches "./" not just ".".
		},
		{
			name:     "double dot",
			uri:      "..",
			expected: false, // Only matches "../" not just "..".
		},
		{
			name:     "path starting with dot but not ./",
			uri:      ".config/settings",
			expected: false,
		},
		{
			name:     "empty string",
			uri:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasLocalPathPrefix(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSchemeSeparator(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Scheme separators present.
		{
			name:     "https scheme",
			uri:      "https://github.com/owner/repo.git",
			expected: true,
		},
		{
			name:     "http scheme",
			uri:      "http://example.com/path",
			expected: true,
		},
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/owner/repo.git",
			expected: true,
		},
		{
			name:     "s3:: prefix",
			uri:      "s3::https://s3.amazonaws.com/bucket/key",
			expected: true,
		},
		{
			name:     "ssh scheme",
			uri:      "ssh://git@github.com/owner/repo.git",
			expected: true,
		},
		{
			name:     "file scheme",
			uri:      "file:///absolute/path",
			expected: true,
		},
		{
			name:     "oci scheme",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: true,
		},
		// go-getter subdirectory delimiter (not a scheme).
		{
			name:     "subdirectory delimiter only",
			uri:      "github.com/owner/repo.git//modules/vpc",
			expected: false, // Has // but not :// or ::, so no scheme separator.
		},
		// No scheme separators.
		{
			name:     "implicit https",
			uri:      "github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "local path",
			uri:      "./relative/path",
			expected: false,
		},
		{
			name:     "absolute path",
			uri:      "/absolute/path",
			expected: false,
		},
		{
			name:     "relative path",
			uri:      "components/terraform/vpc",
			expected: false,
		},
		// Edge cases.
		{
			name:     "colon but not scheme separator",
			uri:      "host:port/path",
			expected: false, // Port notation, not a scheme.
		},
		{
			name:     "empty string",
			uri:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSchemeSeparator(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

//nolint:dupl // Test cases are similar to TestContainsTripleSlash but test different function behavior.
func TestHasSubdirectoryDelimiter(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// With subdirectory delimiter.
		{
			name:     "github with subdirectory",
			uri:      "github.com/owner/repo.git//modules/vpc",
			expected: true,
		},
		{
			name:     "github with root directory",
			uri:      "github.com/owner/repo.git//.",
			expected: true,
		},
		{
			name:     "https with subdirectory",
			uri:      "https://github.com/owner/repo.git//path",
			expected: true,
		},
		{
			name:     "triple-slash pattern",
			uri:      "github.com/owner/repo.git///?ref=v1.0",
			expected: true,
		},
		{
			name:     "git:: with subdirectory",
			uri:      "git::https://github.com/owner/repo.git//examples",
			expected: true,
		},
		// Without subdirectory delimiter.
		{
			name:     "https without subdirectory",
			uri:      "https://github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "implicit https",
			uri:      "github.com/owner/repo.git?ref=main",
			expected: false,
		},
		{
			name:     "local path",
			uri:      "./relative/path",
			expected: false,
		},
		{
			name:     "file scheme",
			uri:      "file:///absolute/path",
			expected: false,
		},
		// Edge cases.
		{
			name:     "path with double slash but not delimiter",
			uri:      "http://example.com/path",
			expected: false, // The // is part of the scheme, not a delimiter.
		},
		{
			name:     "oci scheme",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: false, // OCI uses :// not //.
		},
		{
			name:     "empty string",
			uri:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSubdirectoryDelimiter(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGitURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Git URIs - known Git hosting platforms.
		{
			name:     "github.com URL",
			uri:      "github.com/cloudposse/atmos.git",
			expected: true,
		},
		{
			name:     "gitlab.com URL",
			uri:      "gitlab.com/group/project.git",
			expected: true,
		},
		{
			name:     "bitbucket.org URL",
			uri:      "bitbucket.org/owner/repo.git",
			expected: true,
		},
		{
			name:     "https github URL",
			uri:      "https://github.com/cloudposse/atmos.git",
			expected: true,
		},
		{
			name:     "git:: explicit prefix",
			uri:      "git::https://github.com/cloudposse/atmos.git",
			expected: true,
		},
		// Git URIs - .git extension.
		{
			name:     "URL with .git extension",
			uri:      "example.com/path/repo.git",
			expected: true,
		},
		{
			name:     ".git followed by slash",
			uri:      "git.company.com/repo.git/path",
			expected: true,
		},
		{
			name:     ".git followed by query",
			uri:      "git.company.com/repo.git?ref=main",
			expected: true,
		},
		{
			name:     ".git at end of URL",
			uri:      "git.company.com/repo.git",
			expected: true,
		},
		// Git URIs - Azure DevOps.
		{
			name:     "Azure DevOps URL",
			uri:      "dev.azure.com/org/project/_git/repo",
			expected: true,
		},
		{
			name:     "Azure DevOps with subdirectory",
			uri:      "dev.azure.com/org/project/_git/repo//modules",
			expected: true,
		},
		// Not Git URIs - false positives to avoid.
		{
			name:     "www.gitman.com should not match",
			uri:      "www.gitman.com/page",
			expected: false,
		},
		{
			name:     ".git in middle of word",
			uri:      "example.com/digit.github.io/page",
			expected: true, // Contains .git so it matches.
		},
		{
			name:     "github in path, not domain",
			uri:      "evil.com/github.com/fake",
			expected: false,
		},
		// Not Git URIs - local paths.
		{
			name:     "simple relative path",
			uri:      "components/terraform/vpc",
			expected: false,
		},
		{
			name:     "local absolute path",
			uri:      "/absolute/path/to/components",
			expected: false,
		},
		// Not Git URIs - other schemes.
		{
			name:     "http archive URL",
			uri:      "https://example.com/archive.tar.gz",
			expected: false,
		},
		{
			name:     "oci registry",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: false,
		},
		{
			name:     "s3 URL",
			uri:      "s3::https://s3.amazonaws.com/bucket/key",
			expected: false,
		},
		// Security / Malicious Edge Cases.
		{
			name:     "path traversal in URL",
			uri:      "github.com/owner/repo/../../../etc/passwd",
			expected: true, // Still a Git URL, path traversal handled by downloader.
		},
		{
			name:     "null bytes in URL (Go's url.Parse handles this)",
			uri:      "github.com/owner/repo\x00/malicious",
			expected: false, // URL parsing fails, returns false.
		},
		{
			name:     "unicode homograph attack - gιthub.com (Greek iota)",
			uri:      "gιthub.com/owner/repo",
			expected: false, // Not actual github.com.
		},
		{
			name:     "double scheme exploitation attempt",
			uri:      "git::git::https://github.com/owner/repo",
			expected: true, // Has git:: prefix.
		},
		{
			name:     "file:// with git patterns to trick detection",
			uri:      "file:///tmp/fake.git",
			expected: true, // Has .git in path, but file:// scheme prevents actual Git clone.
		},
		{
			name:     "javascript: pseudo-protocol",
			uri:      "javascript:alert('XSS').git",
			expected: false, // Not a Git URL.
		},
		{
			name:     "data: URL with .git",
			uri:      "data:text/html,<script>alert('XSS')</script>.git",
			expected: false, // Not a Git URL.
		},
		{
			name:     "extremely long URL to test DoS",
			uri:      "github.com/" + strings.Repeat("a", 10000) + "/repo.git",
			expected: true, // Still valid Git URL, length handled elsewhere.
		},
		{
			name:     "URL with credentials in path segment",
			uri:      "evil.com/https://user:pass@github.com/fake",
			expected: false, // Not actual GitHub in host.
		},
		{
			name:     "Mixed case attack - GiThUb.CoM",
			uri:      "GiThUb.CoM/owner/repo",
			expected: true, // Case-insensitive matching.
		},
		{
			name:     "Subdomain confusion - evil-github.com",
			uri:      "evil-github.com/owner/repo.git",
			expected: true, // Has .git extension, would attempt Git clone.
		},
		{
			name:     "Port number in URL (url.Parse handles correctly)",
			uri:      "github.com:22/owner/repo.git",
			expected: true, // url.Parse treats :22 as port, still github.com host with .git.
		},
		{
			name:     "SCP-style Git URL (not standard HTTP URL)",
			uri:      "git@github.com:owner/repo.git",
			expected: true, // SCP-style detected via regex pattern before url.Parse.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGitURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDomainLikeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Domain-like structures (hostname.domain/path).
		{
			name:     "self-hosted git server",
			uri:      "git.company.com/team/repo",
			expected: true,
		},
		{
			name:     "gitea instance",
			uri:      "gitea.company.io/owner/repo",
			expected: true,
		},
		{
			name:     "github.com (common case)",
			uri:      "github.com/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "gitlab.com",
			uri:      "gitlab.com/group/project",
			expected: true,
		},
		{
			name:     "custom domain with path",
			uri:      "code.example.org/path/to/repo",
			expected: true,
		},
		// Not domain-like.
		{
			name:     "no dot in URI",
			uri:      "localhost/path",
			expected: false,
		},
		{
			name:     "dot at beginning",
			uri:      ".hidden/file",
			expected: false,
		},
		{
			name:     "dot at end with no slash",
			uri:      "example.com",
			expected: false, // No slash after domain.
		},
		{
			name:     "relative path with ../",
			uri:      "../parent/path",
			expected: false,
		},
		{
			name:     "relative path with ./",
			uri:      "./current/path",
			expected: false,
		},
		{
			name:     "absolute path",
			uri:      "/absolute/path",
			expected: false,
		},
		{
			name:     "domain without path (no slash after domain)",
			uri:      "example.com",
			expected: false,
		},
		{
			name:     "file extension only",
			uri:      "file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDomainLikeURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Local paths - with prefix.
		{
			name:     "absolute unix path",
			uri:      "/absolute/path/to/components",
			expected: true,
		},
		{
			name:     "relative path with ./",
			uri:      "./relative/path",
			expected: true,
		},
		{
			name:     "parent path with ../",
			uri:      "../parent/path",
			expected: true,
		},
		// Local paths - without prefix (relative).
		{
			name:     "relative path without prefix",
			uri:      "components/terraform/vpc",
			expected: true,
		},
		{
			name:     "nested relative path",
			uri:      "mixins/context.tf",
			expected: true,
		},
		{
			name:     "single directory",
			uri:      "components",
			expected: true,
		},
		// Remote paths - scheme separators.
		{
			name:     "https scheme",
			uri:      "https://github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "ssh scheme",
			uri:      "ssh://git@github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "file scheme",
			uri:      "file:///absolute/path",
			expected: false,
		},
		{
			name:     "oci scheme",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: false,
		},
		// Remote paths - go-getter subdirectory delimiter.
		{
			name:     "github with subdirectory delimiter",
			uri:      "github.com/owner/repo.git//modules/vpc",
			expected: false,
		},
		{
			name:     "self-hosted git with delimiter",
			uri:      "git.company.com/repo.git//path",
			expected: false,
		},
		// Remote paths - Git URIs.
		{
			name:     "github URL",
			uri:      "github.com/cloudposse/atmos.git",
			expected: false,
		},
		{
			name:     "gitlab URL",
			uri:      "gitlab.com/group/project.git",
			expected: false,
		},
		{
			name:     "bitbucket URL",
			uri:      "bitbucket.org/owner/repo.git",
			expected: false,
		},
		{
			name:     "URL with .git extension",
			uri:      "git.company.com/team/repo.git",
			expected: false,
		},
		{
			name:     "Azure DevOps URL",
			uri:      "dev.azure.com/org/project/_git/repo",
			expected: false,
		},
		// Domain-like URIs.
		{
			name:     "self-hosted git server",
			uri:      "git.company.com/team/repo",
			expected: false,
		},
		{
			name:     "gitea instance",
			uri:      "gitea.company.io/owner/repo",
			expected: false,
		},
		{
			name:     "custom domain with path",
			uri:      "code.example.org/path/to/repo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLocalPath(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

//nolint:dupl // Test cases are similar to TestHasSubdirectoryDelimiter but test different function behavior.
func TestContainsTripleSlash(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Contains triple-slash.
		{
			name:     "triple-slash at end",
			uri:      "github.com/owner/repo.git///?ref=v1.0",
			expected: true,
		},
		{
			name:     "triple-slash with path",
			uri:      "github.com/owner/repo.git///modules?ref=v1.0",
			expected: true,
		},
		{
			name:     "triple-slash with subdirectory",
			uri:      "github.com/owner/repo.git///path/to/subdir",
			expected: true,
		},
		{
			name:     "https with triple-slash",
			uri:      "https://github.com/owner/repo.git///?ref=main",
			expected: true,
		},
		{
			name:     "git:: with triple-slash",
			uri:      "git::https://github.com/owner/repo.git///examples",
			expected: true,
		},
		// Does not contain triple-slash.
		{
			name:     "double-slash only",
			uri:      "github.com/owner/repo.git//modules",
			expected: false,
		},
		{
			name:     "double-slash-dot",
			uri:      "github.com/owner/repo.git//.?ref=v1.0",
			expected: false,
		},
		{
			name:     "scheme-only (no triple-slash)",
			uri:      "https://github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "no delimiter",
			uri:      "github.com/owner/repo.git",
			expected: false,
		},
		{
			name:     "file scheme",
			uri:      "file:///absolute/path",
			expected: true, // File URIs contain triple-slash as part of the scheme.
		},
		{
			name:     "local path",
			uri:      "./relative/path",
			expected: false,
		},
		{
			name:     "empty string",
			uri:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsTripleSlash(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSubdirFromTripleSlash(t *testing.T) {
	tests := []struct {
		name           string
		uri            string
		expectedSource string
		expectedSubdir string
	}{
		// Triple-slash with subdirectory.
		{
			name:           "triple-slash with modules path",
			uri:            "github.com/owner/repo.git///modules?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "modules",
		},
		{
			name:           "triple-slash with nested path",
			uri:            "github.com/owner/repo.git///path/to/subdir?ref=main",
			expectedSource: "github.com/owner/repo.git?ref=main",
			expectedSubdir: "path/to/subdir",
		},
		{
			name:           "https with triple-slash path",
			uri:            "https://github.com/owner/repo.git///examples?ref=v1.0",
			expectedSource: "https://github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "examples",
		},
		{
			name:           "Azure DevOps with triple-slash modules path (DEV-3639 regression)",
			uri:            "https://dev.azure.com/org/proj/_git/repo///modules?ref=main",
			expectedSource: "https://dev.azure.com/org/proj/_git/repo?ref=main",
			expectedSubdir: "modules",
		},
		// Triple-slash at root (no path after ///).
		{
			name:           "triple-slash at root with query",
			uri:            "github.com/owner/repo.git///?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "",
		},
		{
			name:           "triple-slash at end no query",
			uri:            "github.com/owner/repo.git///",
			expectedSource: "github.com/owner/repo.git",
			expectedSubdir: "",
		},
		// Double-slash patterns (should not have leading / in subdir).
		{
			name:           "double-slash-dot",
			uri:            "github.com/owner/repo.git//.?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: ".",
		},
		{
			name:           "double-slash with path",
			uri:            "github.com/owner/repo.git//modules?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "modules",
		},
		// Edge cases.
		{
			name:           "no delimiter",
			uri:            "github.com/owner/repo.git?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "",
		},
		{
			name:           "empty string",
			uri:            "",
			expectedSource: "",
			expectedSubdir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, subdir := ParseSubdirFromTripleSlash(tt.uri)
			assert.Equal(t, tt.expectedSource, source)
			assert.Equal(t, tt.expectedSubdir, subdir)
		})
	}
}

func TestNeedsDoubleSlashDot(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Git URIs without subdirectory - needs //.
		{
			name:     "github without subdir",
			uri:      "github.com/owner/repo.git?ref=v1.0",
			expected: true,
		},
		{
			name:     "gitlab without subdir",
			uri:      "gitlab.com/group/project.git?ref=main",
			expected: true,
		},
		{
			name:     "self-hosted git without subdir",
			uri:      "git.company.com/team/repo.git?ref=v1.0",
			expected: true,
		},
		{
			name:     "https git without subdir",
			uri:      "https://github.com/owner/repo.git?ref=v1.0",
			expected: true,
		},
		{
			name:     "git:: without subdir",
			uri:      "git::https://github.com/owner/repo.git?ref=main",
			expected: true,
		},
		{
			name:     "azure devops without subdir",
			uri:      "dev.azure.com/org/project/_git/repo?ref=main",
			expected: true,
		},
		// Git URIs with subdirectory - already has //.
		{
			name:     "github with subdirectory",
			uri:      "github.com/owner/repo.git//modules?ref=v1.0",
			expected: false,
		},
		{
			name:     "github with double-slash-dot",
			uri:      "github.com/owner/repo.git//.?ref=v1.0",
			expected: false,
		},
		{
			name:     "git:: with subdirectory",
			uri:      "git::https://github.com/owner/repo.git//examples?ref=main",
			expected: false,
		},
		// Not Git URIs - should not add //.
		{
			name:     "local relative path",
			uri:      "./components/terraform/vpc",
			expected: false,
		},
		{
			name:     "local absolute path",
			uri:      "/absolute/path/to/components",
			expected: false,
		},
		{
			name:     "file:// URI",
			uri:      "file:///absolute/path",
			expected: false,
		},
		{
			name:     "oci registry",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: false,
		},
		{
			name:     "s3 URL",
			uri:      "s3::https://s3.amazonaws.com/bucket/key",
			expected: false,
		},
		{
			name:     "http archive",
			uri:      "https://example.com/archive.tar.gz",
			expected: false,
		},
		// Special case: URIs that pass IsGitURI() but are special types (lines 243-245).
		{
			name:     "file:// with .git pattern",
			uri:      "file:///tmp/repo.git",
			expected: false, // Has .git but file:// URIs should not get //.
		},
		{
			name:     "github archive download URL",
			uri:      "https://github.com/cloudposse/atmos/archive/refs/tags/v1.0.tar.gz",
			expected: false, // Contains github.com but is an archive, not a Git repo.
		},
		{
			name:     "github release tarball",
			uri:      "https://github.com/owner/repo/releases/download/v1.0/package.tgz",
			expected: false, // Contains github.com but is a release archive, not Git.
		},
		{
			name:     "gitlab archive URL with .git in path",
			uri:      "https://gitlab.com/group/project/-/archive/main/project.tar.gz",
			expected: false, // Contains gitlab.com but is an archive URL.
		},
		// Edge cases.
		{
			name:     "empty string",
			uri:      "",
			expected: false,
		},
		{
			name:     "SCP-style Git URL",
			uri:      "git@github.com:owner/repo.git",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NeedsDoubleSlashDot(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendDoubleSlashDot(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		// Without query parameters.
		{
			name:     "simple github URL",
			uri:      "github.com/owner/repo.git",
			expected: "github.com/owner/repo.git//.",
		},
		{
			name:     "https URL",
			uri:      "https://github.com/owner/repo.git",
			expected: "https://github.com/owner/repo.git//.",
		},
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/owner/repo.git",
			expected: "git::https://github.com/owner/repo.git//.",
		},
		{
			name:     "self-hosted git",
			uri:      "git.company.com/team/repo.git",
			expected: "git.company.com/team/repo.git//.",
		},
		// With query parameters - should preserve query string.
		{
			name:     "with ref query param",
			uri:      "github.com/owner/repo.git?ref=v1.0",
			expected: "github.com/owner/repo.git//.?ref=v1.0",
		},
		{
			name:     "with multiple query params",
			uri:      "github.com/owner/repo.git?ref=main&depth=1",
			expected: "github.com/owner/repo.git//.?ref=main&depth=1",
		},
		{
			name:     "https with query params",
			uri:      "https://github.com/owner/repo.git?ref=v1.0&depth=1",
			expected: "https://github.com/owner/repo.git//.?ref=v1.0&depth=1",
		},
		{
			name:     "git:: with query params",
			uri:      "git::https://github.com/owner/repo.git?ref=main",
			expected: "git::https://github.com/owner/repo.git//.?ref=main",
		},
		// Edge cases.
		{
			name:     "empty string",
			uri:      "",
			expected: "//.",
		},
		{
			name:     "URI already ending with //",
			uri:      "github.com/org/repo.git//?ref=v1.0",
			expected: "github.com/org/repo.git//.?ref=v1.0",
		},
		{
			name:     "SCP-style URL",
			uri:      "git@github.com:owner/repo.git",
			expected: "git@github.com:owner/repo.git//.",
		},
		{
			name:     "SCP-style with query",
			uri:      "git@github.com:owner/repo.git?ref=v1.0",
			expected: "git@github.com:owner/repo.git//.?ref=v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendDoubleSlashDot(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}
