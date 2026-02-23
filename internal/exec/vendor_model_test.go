package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestVendorErrorMessage_DescriptiveExplanation(t *testing.T) {
	// Regression test: the vendor error must contain a descriptive explanation
	// listing the failed component names, not just a bare integer count.
	t.Run("single failed component", func(t *testing.T) {
		failedPkgNames := []string{"my-vpc"}
		totalPkgs := 3
		failedPkg := 1

		explanation := fmt.Sprintf("Failed to vendor %d of %d components: %s",
			failedPkg, totalPkgs, strings.Join(failedPkgNames, ", "))
		err := errUtils.Build(ErrVendorComponents).
			WithExplanation(explanation).
			Err()

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.Contains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "Failed to vendor 1 of 3 components")
	})

	t.Run("multiple failed components", func(t *testing.T) {
		failedPkgNames := []string{"my-vpc", "my-rds", "my-s3"}
		totalPkgs := 5
		failedPkg := 3

		explanation := fmt.Sprintf("Failed to vendor %d of %d components: %s",
			failedPkg, totalPkgs, strings.Join(failedPkgNames, ", "))
		err := errUtils.Build(ErrVendorComponents).
			WithExplanation(explanation).
			Err()

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
