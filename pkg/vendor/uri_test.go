package vendor

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
		{name: "absolute unix path", uri: "/absolute/path/to/components", expected: true},
		{name: "current directory prefix", uri: "./relative/path", expected: true},
		{name: "parent directory prefix", uri: "../parent/path", expected: true},
		{name: "github URL", uri: "github.com/owner/repo.git", expected: false},
		{name: "https URL", uri: "https://github.com/owner/repo.git", expected: false},
		{name: "relative path without prefix", uri: "components/terraform/vpc", expected: false},
		{name: "single dot", uri: ".", expected: false},
		{name: "double dot", uri: "..", expected: false},
		{name: "dot prefix without slash", uri: ".config/settings", expected: false},
		{name: "empty string", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasLocalPathPrefix(tt.uri))
		})
	}
}

//nolint:dupl // Table-driven tests for different URI functions have similar structure by design.
func TestHasSchemeSeparator(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{name: "https scheme", uri: "https://github.com/owner/repo.git", expected: true},
		{name: "http scheme", uri: "http://example.com/path", expected: true},
		{name: "git:: prefix", uri: "git::https://github.com/owner/repo.git", expected: true},
		{name: "s3:: prefix", uri: "s3::https://s3.amazonaws.com/bucket/key", expected: true},
		{name: "ssh scheme", uri: "ssh://git@github.com/owner/repo.git", expected: true},
		{name: "file scheme", uri: "file:///absolute/path", expected: true},
		{name: "oci scheme", uri: "oci://ghcr.io/owner/image:tag", expected: true},
		{name: "subdirectory delimiter only", uri: "github.com/owner/repo.git//modules/vpc", expected: false},
		{name: "implicit https", uri: "github.com/owner/repo.git", expected: false},
		{name: "local path", uri: "./relative/path", expected: false},
		{name: "colon but not scheme", uri: "host:port/path", expected: false},
		{name: "empty string", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasSchemeSeparator(tt.uri))
		})
	}
}

//nolint:dupl // Table-driven tests for different URI functions have similar structure by design.
func TestHasSubdirectoryDelimiter(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{name: "github with subdirectory", uri: "github.com/owner/repo.git//modules/vpc", expected: true},
		{name: "github with root directory", uri: "github.com/owner/repo.git//.", expected: true},
		{name: "https with subdirectory", uri: "https://github.com/owner/repo.git//path", expected: true},
		{name: "triple-slash pattern", uri: "github.com/owner/repo.git///?ref=v1.0", expected: true},
		{name: "git:: with subdirectory", uri: "git::https://github.com/owner/repo.git//examples", expected: true},
		{name: "https without subdirectory", uri: "https://github.com/owner/repo.git", expected: false},
		{name: "implicit https", uri: "github.com/owner/repo.git?ref=main", expected: false},
		{name: "local path", uri: "./relative/path", expected: false},
		{name: "file scheme", uri: "file:///absolute/path", expected: false},
		{name: "http double slash is scheme not delimiter", uri: "http://example.com/path", expected: false},
		{name: "oci scheme", uri: "oci://ghcr.io/owner/image:tag", expected: false},
		{name: "empty string", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasSubdirectoryDelimiter(tt.uri))
		})
	}
}

func TestIsGitURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Known Git hosts.
		{name: "github.com", uri: "github.com/cloudposse/atmos.git", expected: true},
		{name: "gitlab.com", uri: "gitlab.com/group/project.git", expected: true},
		{name: "bitbucket.org", uri: "bitbucket.org/owner/repo.git", expected: true},
		{name: "https github", uri: "https://github.com/cloudposse/atmos.git", expected: true},
		{name: "git:: prefix", uri: "git::https://github.com/owner/repo.git", expected: true},
		// .git extension.
		{name: ".git extension", uri: "example.com/path/repo.git", expected: true},
		{name: ".git with query", uri: "git.company.com/repo.git?ref=main", expected: true},
		// Azure DevOps.
		{name: "Azure DevOps", uri: "dev.azure.com/org/project/_git/repo", expected: true},
		// SCP-style.
		{name: "SCP-style", uri: "git@github.com:owner/repo.git", expected: true},
		// Not Git.
		{name: "simple relative path", uri: "components/terraform/vpc", expected: false},
		{name: "local absolute path", uri: "/absolute/path/to/components", expected: false},
		{name: "oci registry", uri: "oci://ghcr.io/owner/image:tag", expected: false},
		{name: "s3 URL", uri: "s3::https://s3.amazonaws.com/bucket/key", expected: false},
		{name: "http archive", uri: "https://example.com/archive.tar.gz", expected: false},
		// Security edge cases.
		{name: "unicode homograph", uri: "gιthub.com/owner/repo", expected: false},
		{name: "mixed case github", uri: "GiThUb.CoM/owner/repo", expected: true},
		{name: "extremely long URL", uri: "github.com/" + strings.Repeat("a", 10000) + "/repo.git", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsGitURI(tt.uri))
		})
	}
}

func TestIsDomainLikeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{name: "self-hosted git", uri: "git.company.com/team/repo", expected: true},
		{name: "github.com", uri: "github.com/cloudposse/atmos", expected: true},
		{name: "no dot", uri: "localhost/path", expected: false},
		{name: "dot at beginning", uri: ".hidden/file", expected: false},
		{name: "no slash after domain", uri: "example.com", expected: false},
		{name: "relative path", uri: "../parent/path", expected: false},
		{name: "absolute path", uri: "/absolute/path", expected: false},
		{name: "file extension only", uri: "file.txt", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsDomainLikeURI(tt.uri))
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Local paths.
		{name: "absolute unix path", uri: "/absolute/path/to/components", expected: true},
		{name: "relative with ./", uri: "./relative/path", expected: true},
		{name: "parent with ../", uri: "../parent/path", expected: true},
		{name: "relative without prefix", uri: "components/terraform/vpc", expected: true},
		{name: "nested relative", uri: "mixins/context.tf", expected: true},
		{name: "single directory", uri: "components", expected: true},
		// Remote.
		{name: "https scheme", uri: "https://github.com/owner/repo.git", expected: false},
		{name: "git:: prefix", uri: "git::https://github.com/owner/repo.git", expected: false},
		{name: "file scheme", uri: "file:///absolute/path", expected: false},
		{name: "oci scheme", uri: "oci://ghcr.io/owner/image:tag", expected: false},
		{name: "github with subdir", uri: "github.com/owner/repo.git//modules/vpc", expected: false},
		{name: "github URL", uri: "github.com/cloudposse/atmos.git", expected: false},
		{name: "domain-like", uri: "git.company.com/team/repo", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsLocalPath(tt.uri))
		})
	}
}

func TestContainsTripleSlash(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{name: "triple-slash at end", uri: "github.com/owner/repo.git///?ref=v1.0", expected: true},
		{name: "triple-slash with path", uri: "github.com/owner/repo.git///modules?ref=v1.0", expected: true},
		{name: "https with triple-slash", uri: "https://github.com/owner/repo.git///?ref=main", expected: true},
		{name: "file scheme", uri: "file:///absolute/path", expected: true},
		{name: "double-slash only", uri: "github.com/owner/repo.git//modules", expected: false},
		{name: "double-slash-dot", uri: "github.com/owner/repo.git//.?ref=v1.0", expected: false},
		{name: "no delimiter", uri: "github.com/owner/repo.git", expected: false},
		{name: "empty string", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ContainsTripleSlash(tt.uri))
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
			name:           "triple-slash at root with query",
			uri:            "github.com/owner/repo.git///?ref=v1.0",
			expectedSource: "github.com/owner/repo.git?ref=v1.0",
			expectedSubdir: "",
		},
		{
			name:           "Azure DevOps with triple-slash",
			uri:            "https://dev.azure.com/org/proj/_git/repo///modules?ref=main",
			expectedSource: "https://dev.azure.com/org/proj/_git/repo?ref=main",
			expectedSubdir: "modules",
		},
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

//nolint:dupl // Table-driven tests for different URI functions have similar structure by design.
func TestNeedsDoubleSlashDot(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Needs //.
		{name: "github without subdir", uri: "github.com/owner/repo.git?ref=v1.0", expected: true},
		{name: "https git without subdir", uri: "https://github.com/owner/repo.git?ref=v1.0", expected: true},
		{name: "git:: without subdir", uri: "git::https://github.com/owner/repo.git?ref=main", expected: true},
		{name: "SCP-style", uri: "git@github.com:owner/repo.git", expected: true},
		// Already has subdir.
		{name: "github with subdirectory", uri: "github.com/owner/repo.git//modules?ref=v1.0", expected: false},
		{name: "github with double-slash-dot", uri: "github.com/owner/repo.git//.?ref=v1.0", expected: false},
		// Not Git.
		{name: "local relative path", uri: "./components/terraform/vpc", expected: false},
		{name: "file:// URI", uri: "file:///absolute/path", expected: false},
		{name: "oci registry", uri: "oci://ghcr.io/owner/image:tag", expected: false},
		{name: "s3 URL", uri: "s3::https://s3.amazonaws.com/bucket/key", expected: false},
		{name: "http archive", uri: "https://example.com/archive.tar.gz", expected: false},
		{name: "empty string", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NeedsDoubleSlashDot(tt.uri))
		})
	}
}

func TestAppendDoubleSlashDot(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{name: "simple github URL", uri: "github.com/owner/repo.git", expected: "github.com/owner/repo.git//."},
		{name: "https URL", uri: "https://github.com/owner/repo.git", expected: "https://github.com/owner/repo.git//."},
		{name: "git:: prefix", uri: "git::https://github.com/owner/repo.git", expected: "git::https://github.com/owner/repo.git//."},
		{name: "with ref query param", uri: "github.com/owner/repo.git?ref=v1.0", expected: "github.com/owner/repo.git//.?ref=v1.0"},
		{name: "with multiple query params", uri: "github.com/owner/repo.git?ref=main&depth=1", expected: "github.com/owner/repo.git//.?ref=main&depth=1"},
		{name: "SCP-style URL", uri: "git@github.com:owner/repo.git", expected: "git@github.com:owner/repo.git//."},
		{name: "SCP-style with query", uri: "git@github.com:owner/repo.git?ref=v1.0", expected: "git@github.com:owner/repo.git//.?ref=v1.0"},
		{name: "empty string", uri: "", expected: "//."},
		{name: "URI ending with //", uri: "github.com/org/repo.git//?ref=v1.0", expected: "github.com/org/repo.git//.?ref=v1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, AppendDoubleSlashDot(tt.uri))
		})
	}
}

func TestNormalizeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "triple slash with subdir",
			uri:      "github.com/cloudposse/terraform-aws-vpc///modules/vpc?ref=1.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//modules/vpc?ref=1.0",
		},
		{
			name:     "triple slash root",
			uri:      "github.com/cloudposse/terraform-aws-vpc///?ref=1.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//.?ref=1.0",
		},
		{
			name:     "git URL without subdir gets double-slash-dot",
			uri:      "github.com/cloudposse/terraform-aws-vpc?ref=1.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//.?ref=1.0",
		},
		{
			name:     "already has subdirectory",
			uri:      "github.com/cloudposse/terraform-aws-vpc//modules/vpc?ref=1.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//modules/vpc?ref=1.0",
		},
		{
			name:     "local path unchanged",
			uri:      "./components/terraform/vpc",
			expected: "./components/terraform/vpc",
		},
		{
			name:     "OCI URI unchanged",
			uri:      "oci://ghcr.io/owner/image:tag",
			expected: "oci://ghcr.io/owner/image:tag",
		},
		{
			name:     "S3 URI unchanged",
			uri:      "s3::https://s3.amazonaws.com/bucket/key",
			expected: "s3::https://s3.amazonaws.com/bucket/key",
		},
		{
			name:     "file URI unchanged",
			uri:      "file:///absolute/path",
			expected: "file:///absolute/path",
		},
		{
			name:     "http archive unchanged",
			uri:      "https://example.com/archive.tar.gz",
			expected: "https://example.com/archive.tar.gz",
		},
		{
			name:     "empty URI",
			uri:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeURI(tt.uri))
		})
	}
}

func TestIsFileURI(t *testing.T) {
	assert.True(t, IsFileURI("file:///absolute/path"))
	assert.False(t, IsFileURI("https://example.com"))
	assert.False(t, IsFileURI(""))
}

func TestIsOCIURI(t *testing.T) {
	assert.True(t, IsOCIURI("oci://ghcr.io/owner/image:tag"))
	assert.False(t, IsOCIURI("https://example.com"))
	assert.False(t, IsOCIURI(""))
}

func TestIsS3URI(t *testing.T) {
	assert.True(t, IsS3URI("s3::https://s3.amazonaws.com/bucket/key"))
	assert.True(t, IsS3URI("https://s3.amazonaws.com/bucket/key"))
	assert.False(t, IsS3URI("https://example.com"))
	assert.False(t, IsS3URI(""))
}

func TestIsNonGitHTTPURI(t *testing.T) {
	assert.True(t, IsNonGitHTTPURI("https://example.com/archive.tar.gz"))
	assert.True(t, IsNonGitHTTPURI("https://example.com/archive.zip"))
	assert.True(t, IsNonGitHTTPURI("http://example.com/file.tgz"))
	assert.False(t, IsNonGitHTTPURI("https://github.com/owner/repo.git"))
	assert.False(t, IsNonGitHTTPURI("github.com/owner/repo"))
	assert.False(t, IsNonGitHTTPURI(""))
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{name: "simple path", uri: "https://example.com/file.tf", expected: "file.tf"},
		{name: "nested path", uri: "https://example.com/path/to/module.tar.gz", expected: "module.tar.gz"},
		{name: "git URL", uri: "github.com/owner/repo.git", expected: "repo.git"},
		{name: "query params stripped", uri: "https://example.com/module?ref=v1", expected: "module"},
		{name: "invalid URI fallback", uri: "://invalid", expected: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, SanitizeFileName(tt.uri))
		})
	}
}
