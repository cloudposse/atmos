package imports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Local paths.
		{"absolute path", "/absolute/path", true},
		{"relative dot path", "./relative/path", true},
		{"relative parent path", "../parent/path", true},
		{"simple relative path", "components/terraform", true},
		{"catalog path", "catalog/vpc", true},
		{"mixins path", "mixins/region/us-east-2", true},

		// Remote URLs - HTTP/HTTPS.
		{"https URL", "https://example.com/config.yaml", false},
		{"http URL", "http://example.com/config.yaml", false},
		{"raw GitHub URL", "https://raw.githubusercontent.com/org/repo/main/config.yaml", false},

		// Remote URLs - Git.
		{"git prefix", "git::https://github.com/org/repo.git", false},
		{"github.com shorthand", "github.com/org/repo//path", false},
		{"gitlab.com", "gitlab.com/org/repo//path", false},
		{"bitbucket.org", "bitbucket.org/org/repo//path", false},
		{"scp style git", "git@github.com:org/repo.git", false},
		{"git URL with ref", "github.com/org/repo//path?ref=v1.0", false},

		// Remote URLs - Cloud storage.
		{"s3 prefix", "s3::https://s3.amazonaws.com/bucket/key", false},
		{"s3 amazonaws URL", "https://s3.amazonaws.com/bucket/key", false},
		{"gcs prefix", "gcs::bucket/path", false},

		// Remote URLs - Domain-like.
		{"custom git host", "git.company.com/repo/path", false},
		{"gitea", "gitea.io/owner/repo/path", false},

		// Edge cases.
		{"file with extension", "catalog/vpc.yaml", true},
		{"file with dots in name", "path/to/file.config.yaml", true},
		{"azure devops", "dev.azure.com/org/project/_git/repo", false},

		// Version-like paths (should be local, not mistaken for domains).
		{"version path v1.0", "configs/v1.0/base", true},
		{"version path v2.1.3", "stacks/v2.1.3/deploy", true},
		{"version path numeric", "releases/1.0/stable", true},
		{"nested version path", "catalog/modules/v3.2/vpc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLocalPath(tt.uri)
			assert.Equal(t, tt.expected, result, "IsLocalPath(%q)", tt.uri)
		})
	}
}

func TestIsRemote(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Local paths (should return false).
		{"local absolute", "/path/to/file.yaml", false},
		{"local relative", "catalog/vpc.yaml", false},
		{"local dot relative", "./config.yaml", false},

		// Remote URLs (should return true).
		{"https", "https://example.com/config.yaml", true},
		{"github shorthand", "github.com/org/repo//path", true},
		{"git prefix", "git::https://github.com/org/repo.git", true},
		{"s3", "s3::https://s3.amazonaws.com/bucket/key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRemote(tt.uri)
			assert.Equal(t, tt.expected, result, "IsRemote(%q)", tt.uri)
		})
	}
}

func TestHasSchemeSeparator(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{"https scheme", "https://example.com", true},
		{"http scheme", "http://example.com", true},
		{"git double colon", "git::https://github.com/org/repo", true},
		{"s3 double colon", "s3::https://bucket/key", true},
		{"file scheme", "file:///path/to/file", true},
		{"no scheme local", "catalog/vpc.yaml", false},
		{"domain without scheme", "github.com/org/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSchemeSeparator(tt.uri)
			assert.Equal(t, tt.expected, result, "HasSchemeSeparator(%q)", tt.uri)
		})
	}
}

func TestIsGitURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Git URIs.
		{"git prefix", "git::https://github.com/org/repo.git", true},
		{"github.com", "github.com/org/repo", true},
		{"gitlab.com", "gitlab.com/org/repo", true},
		{"bitbucket.org", "bitbucket.org/org/repo", true},
		{"scp style", "git@github.com:org/repo.git", true},
		{"git extension", "https://example.com/repo.git", true},
		{"azure devops", "https://dev.azure.com/org/project/_git/repo", true},

		// Non-Git URIs.
		{"plain https", "https://example.com/file.yaml", false},
		{"s3 url", "s3::https://bucket/key", false},
		{"local path", "catalog/vpc.yaml", false},
		{"tar.gz archive", "https://example.com/archive.tar.gz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGitURI(tt.uri)
			assert.Equal(t, tt.expected, result, "IsGitURI(%q)", tt.uri)
		})
	}
}

func TestIsHTTPURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{"https", "https://example.com/file.yaml", true},
		{"http", "http://example.com/file.yaml", true},
		{"git prefix", "git::https://github.com/org/repo", false},
		{"local path", "catalog/vpc.yaml", false},
		{"github shorthand", "github.com/org/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHTTPURI(tt.uri)
			assert.Equal(t, tt.expected, result, "IsHTTPURI(%q)", tt.uri)
		})
	}
}

func TestIsS3URI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{"s3 prefix", "s3::https://s3.amazonaws.com/bucket/key", true},
		{"amazonaws URL", "https://s3.amazonaws.com/bucket/key", true},
		{"s3 region URL", "https://s3-us-west-2.amazonaws.com/bucket/key", true},
		{"plain https", "https://example.com/file.yaml", false},
		{"local path", "catalog/vpc.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsS3URI(tt.uri)
			assert.Equal(t, tt.expected, result, "IsS3URI(%q)", tt.uri)
		})
	}
}

func TestIsGCSURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{"gcs prefix", "gcs::bucket/path", true},
		{"gcs scheme", "gcs://bucket/path", true},
		{"plain https", "https://storage.googleapis.com/bucket/key", false},
		{"local path", "catalog/vpc.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGCSURI(tt.uri)
			assert.Equal(t, tt.expected, result, "IsGCSURI(%q)", tt.uri)
		})
	}
}
