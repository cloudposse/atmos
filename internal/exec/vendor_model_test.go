package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestVendorFailureError(t *testing.T) {
	// Regression test: the vendor error must contain a descriptive explanation
	// listing the failed component names, not just a bare integer count.
	t.Run("single failed component", func(t *testing.T) {
		err := vendorFailureError(1, 3, []string{"my-vpc"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.Contains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "Failed to vendor 1 of 3 components")
	})

	t.Run("multiple failed components", func(t *testing.T) {
		err := vendorFailureError(3, 5, []string{"my-vpc", "my-rds", "my-s3"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.Contains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "my-rds")
		assert.Contains(t, formatted, "my-s3")
		assert.Contains(t, formatted, "Failed to vendor 3 of 5 components")
	})
}

func TestHandleInstalledPkgMsg_TracksFailedNames(t *testing.T) {
	// Verify that handleInstalledPkgMsg appends failed package names.
	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc"},
			{name: "rds"},
		},
		index: 0,
		isTTY: false,
	}

	// Simulate a failed install message.
	msg := &installedPkgMsg{
		err:  errors.New("download failed"),
		name: "vpc",
	}
	m.handleInstalledPkgMsg(msg)

	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)

	// Simulate a second package succeeding.
	m.index = 1
	msg2 := &installedPkgMsg{name: "rds"}
	m.handleInstalledPkgMsg(msg2)

	// Failed count should not change.
	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)
}
