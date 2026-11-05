package aqua

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLatestVersionPreservesTagWithoutMetadata covers registry metadata failures:
// the GitHub tag remains authoritative instead of guessing a "v" convention.
func TestLatestVersionPreservesTagWithoutMetadata(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "1.2.3", "cli-1.2.3"} {
		t.Run(tag, func(t *testing.T) {
			version := latestVersionFromAquaFixture(t, "example", "tool", tag, "packages: []\n")
			assert.Equal(t, tag, version)
		})
	}
}
