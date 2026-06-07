package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyXTerraformGet(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantURL   string
		cacheable bool
	}{
		{"git source", "git::https://github.com/org/repo.git", "", false},
		{"git ssh", "git@github.com:org/repo.git", "", false},
		{"ssh scheme", "ssh://git@github.com/org/repo.git", "", false},
		{"s3 getter", "s3::https://bucket.s3.amazonaws.com/mod.zip", "", false},
		{"http tarball", "https://example.com/mod-1.0.0.tar.gz", "https://example.com/mod-1.0.0.tar.gz", true},
		{"http zip", "https://example.com/mod.zip", "https://example.com/mod.zip", true},
		{"http tgz with query", "https://example.com/mod.tgz?ref=v1", "https://example.com/mod.tgz?ref=v1", true},
		{"http with subdir not cacheable", "https://example.com/mod.tar.gz//subdir", "", false},
		{"http non-archive", "https://example.com/raw/main", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, ok := classifyXTerraformGet(tt.value)
			assert.Equal(t, tt.cacheable, ok)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestParseProviderArchive(t *testing.T) {
	ref, ok := parseProviderArchive("terraform-provider-aws_5.95.0_linux_amd64.zip")
	assert.True(t, ok)
	assert.Equal(t, "5.95.0", ref.version)
	assert.Equal(t, "linux", ref.os)
	assert.Equal(t, "amd64", ref.arch)

	_, ok = parseProviderArchive("not-a-provider.zip")
	assert.False(t, ok)
}
