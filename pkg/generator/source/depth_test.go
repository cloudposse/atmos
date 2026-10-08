package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestWithDepth(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		depth     int
	}{
		{"github.com/acme/starters//example", "github.com/acme/starters//example?depth=1", 1},
		{"git::ssh://git@example.com/repo.git//example?ref=feature%2Fdemo", "git::ssh://git@example.com/repo.git//example?depth=3&ref=feature%2Fdemo", 3},
		{"git::file:///repo?ref=main", "git::file:///repo?depth=0&ref=main", 0},
		{"github.com/acme/starters?depth=0", "github.com/acme/starters?depth=0", 1},
		{"github.com/acme/starters?depth=2", "github.com/acme/starters?depth=2", 0},
		{"./local", "./local", 1},
		{"https://example.com/archive.zip", "https://example.com/archive.zip", 1},
		{"oci://example.com/starter:v1", "oci://example.com/starter:v1", 1},
	} {
		t.Run(tc.want, func(t *testing.T) {
			got, err := WithDepth(tc.src, tc.depth)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	_, err := WithDepth("github.com/acme/starters", -1)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	_, err = WithDepth("github.com/acme/starters?ref=%invalid", 1)
	require.Error(t, err)
}
