//nolint:dupl // Table-driven tests intentionally have similar structure.
package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasLocalPathPrefix(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Absolute paths
		{
			name:     "absolute path",
			uri:      "/absolute/path",
			expected: true,
		},
		{
			name:     "absolute path with single slash",
			uri:      "/",
			expected: true,
		},
		// Relative paths starting with ./
		{
			name:     "relative path with ./",
			uri:      "./relative/path",
			expected: true,
		},
		{
			name:     "current directory only",
			uri:      "./",
			expected: true,
		},
		// Parent paths starting with ../
		{
			name:     "parent path with ../",
			uri:      "../parent/path",
			expected: true,
		},
		{
			name:     "parent directory only",
			uri:      "../",
			expected: true,
		},
		{
			name:     "multiple parent directories",
			uri:      "../../../components/terraform",
			expected: true,
		},
		// Not local path prefixes
		{
			name:     "github URL without prefix",
			uri:      "github.com/owner/repo",
			expected: false,
		},
		{
			name:     "https URL",
			uri:      "https://github.com/owner/repo",
			expected: false,
		},
		{
			name:     "relative path without prefix",
			uri:      "components/terraform",
			expected: false,
		},
		{
			name:     "git URL",
			uri:      "git@github.com:owner/repo.git",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasLocalPathPrefix(tt.uri)
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
		// URIs with :// scheme separator
		{
			name:     "https scheme",
			uri:      "https://github.com/owner/repo",
			expected: true,
		},
		{
			name:     "http scheme",
			uri:      "http://example.com/path",
			expected: true,
		},
		{
			name:     "file scheme",
			uri:      "file:///path/to/file",
			expected: true,
		},
		{
			name:     "oci scheme",
			uri:      "oci://public.ecr.aws/image:tag",
			expected: true,
		},
		{
			name:     "ssh scheme",
			uri:      "ssh://git@github.com/owner/repo",
			expected: true,
		},
		// URIs with :: scheme separator
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/owner/repo",
			expected: true,
		},
		{
			name:     "s3:: prefix",
			uri:      "s3::https://s3.amazonaws.com/bucket/path",
			expected: true,
		},
		{
			name:     "hg:: prefix (Mercurial)",
			uri:      "hg::https://bitbucket.org/owner/repo",
			expected: true,
		},
		// URIs without scheme separators
		{
			name:     "github URL without scheme",
			uri:      "github.com/owner/repo",
			expected: false,
		},
		{
			name:     "local relative path",
			uri:      "./components/terraform",
			expected: false,
		},
		{
			name:     "local absolute path",
			uri:      "/absolute/path",
			expected: false,
		},
		{
			name:     "simple relative path",
			uri:      "components/terraform",
			expected: false,
		},
		{
			name:     "git URL with subdirectory delimiter",
			uri:      "github.com/repo//path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSchemeSeparator(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSubdirectoryDelimiter(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// URIs with go-getter subdirectory delimiter
		{
			name:     "github with subdirectory path",
			uri:      "github.com/owner/repo//modules/vpc",
			expected: true,
		},
		{
			name:     "github with subdirectory root",
			uri:      "github.com/owner/repo.git//.",
			expected: true,
		},
		{
			name:     "self-hosted git with subdirectory",
			uri:      "git.company.com/repo//infrastructure",
			expected: true,
		},
		{
			name:     "triple-slash pattern (contains //)",
			uri:      "github.com/repo.git///",
			expected: true,
		},
		{
			name:     "https URL with subdirectory",
			uri:      "https://github.com/owner/repo.git//path",
			expected: true,
		},
		// URIs without subdirectory delimiter (but scheme separator contains double-slash)
		{
			name:     "https URL without subdirectory",
			uri:      "https://github.com/owner/repo",
			expected: true, // The scheme separator includes double-slash.
		},
		{
			name:     "file URI",
			uri:      "file:///path/to/file",
			expected: true, // The file scheme includes triple-slash.
		},
		// URIs without any //
		{
			name:     "github URL without subdirectory or scheme",
			uri:      "github.com/owner/repo",
			expected: false,
		},
		{
			name:     "local relative path",
			uri:      "./components/terraform",
			expected: false,
		},
		{
			name:     "local absolute path",
			uri:      "/absolute/path",
			expected: false,
		},
		{
			name:     "simple relative path",
			uri:      "components/terraform/mixins",
			expected: false,
		},
		{
			name:     "git.company.com without subdirectory",
			uri:      "git.company.com/team/repo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSubdirectoryDelimiter(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGitLikeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Common Git hosting platforms
		{
			name:     "github.com URL",
			uri:      "github.com/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "gitlab.com URL",
			uri:      "gitlab.com/project/repo",
			expected: true,
		},
		{
			name:     "bitbucket.org URL",
			uri:      "bitbucket.org/owner/repo",
			expected: true,
		},
		{
			name:     "https github URL",
			uri:      "https://github.com/owner/repo.git",
			expected: true,
		},
		// .git extension
		{
			name:     "URL with .git extension",
			uri:      "git.company.com/repo.git",
			expected: true,
		},
		{
			name:     "local path with .git directory",
			uri:      "/path/to/repo.git",
			expected: true,
		},
		// Azure DevOps pattern
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
		// Not Git-like URIs
		{
			name:     "simple relative path",
			uri:      "components/terraform",
			expected: false,
		},
		{
			name:     "local absolute path",
			uri:      "/absolute/path",
			expected: false,
		},
		{
			name:     "http archive URL",
			uri:      "https://example.com/archive.tar.gz",
			expected: false,
		},
		{
			name:     "oci registry",
			uri:      "oci://public.ecr.aws/image:tag",
			expected: false,
		},
		{
			name:     "s3 URL",
			uri:      "s3::https://s3.amazonaws.com/bucket/path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitLikeURI(tt.uri)
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
		// Domain-like structures (hostname.domain/path)
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
			uri:      "github.com/owner/repo",
			expected: true,
		},
		{
			name:     "gitlab.com",
			uri:      "gitlab.com/project/repo",
			expected: true,
		},
		{
			name:     "custom domain with path",
			uri:      "code.example.org/path/to/repo",
			expected: true,
		},
		// Not domain-like structures
		{
			name:     "no dot in URI",
			uri:      "components/terraform",
			expected: false,
		},
		{
			name:     "dot at beginning",
			uri:      ".hidden/file",
			expected: false,
		},
		{
			name:     "dot at end with no slash",
			uri:      "file.txt",
			expected: false,
		},
		{
			name:     "relative path with ../",
			uri:      "../parent/path",
			expected: false,
		},
		{
			name:     "relative path with ./",
			uri:      "./components/terraform",
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
			uri:      "config.yaml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDomainLikeURI(tt.uri)
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
		// Local paths with explicit prefixes
		{
			name:     "absolute path",
			uri:      "/absolute/path",
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
		{
			name:     "multiple parent directories",
			uri:      "../../../components/terraform",
			expected: true,
		},
		// Simple relative paths (no scheme, no domain structure)
		{
			name:     "simple relative path",
			uri:      "components/terraform",
			expected: true,
		},
		{
			name:     "nested relative path",
			uri:      "mixins/context.tf",
			expected: true,
		},
		// Remote URIs with schemes
		{
			name:     "https URL",
			uri:      "https://github.com/owner/repo",
			expected: false,
		},
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/owner/repo",
			expected: false,
		},
		{
			name:     "s3:: prefix",
			uri:      "s3::https://s3.amazonaws.com/bucket",
			expected: false,
		},
		{
			name:     "file:// URI",
			uri:      "file:///path/to/file",
			expected: false,
		},
		{
			name:     "oci:// URI",
			uri:      "oci://public.ecr.aws/image:tag",
			expected: false,
		},
		// Remote URIs with subdirectory delimiter
		{
			name:     "github with subdirectory",
			uri:      "github.com/owner/repo//modules",
			expected: false,
		},
		{
			name:     "self-hosted git with subdirectory",
			uri:      "git.company.com/repo//path",
			expected: false,
		},
		{
			name:     "triple-slash pattern",
			uri:      "github.com/repo.git///",
			expected: false,
		},
		// Git-like URIs
		{
			name:     "github.com URL",
			uri:      "github.com/cloudposse/atmos",
			expected: false,
		},
		{
			name:     "gitlab.com URL",
			uri:      "gitlab.com/project/repo",
			expected: false,
		},
		{
			name:     "bitbucket.org URL",
			uri:      "bitbucket.org/owner/repo",
			expected: false,
		},
		{
			name:     "URL with .git extension",
			uri:      "git.company.com/repo.git",
			expected: false,
		},
		{
			name:     "Azure DevOps URL",
			uri:      "dev.azure.com/org/project/_git/repo",
			expected: false,
		},
		// Domain-like URIs
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
			result := isLocalPath(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}
