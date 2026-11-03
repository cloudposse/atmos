package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitSourceSupportedTransports(t *testing.T) {
	for _, source := range []string{"git://example.invalid/team/repo?ref=v1", "ssh://git@example.invalid/team/repo.git", "git::https://example.invalid/repo.git", "git::file:///tmp/repo"} {
		t.Run(source, func(t *testing.T) { assert.True(t, IsGitSource(source)) })
	}
	for _, source := range []string{"file:///tmp/repo", "./repo", "https://example.invalid/archive.zip", "oci://registry.invalid/repo"} {
		t.Run(source, func(t *testing.T) { assert.False(t, IsGitSource(source)) })
	}
}
