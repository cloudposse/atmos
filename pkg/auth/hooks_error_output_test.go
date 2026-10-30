package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The pre-hook returns its failures and leaves rendering to the command boundary, keeping the
// actionable guidance as hints so nothing is lost by not printing it here.
func TestTerraformPreHookHelpers_ReturnHintedErrorsWithoutPrinting(t *testing.T) {
	t.Run("no default identity", func(t *testing.T) {
		var err error
		stderr := captureStderr(t, func() {
			_, err = resolveTargetIdentityName(context.Background(), &schema.ConfigAndStacksInfo{}, &stubAuthManager{})
		})

		require.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity)
		assert.Empty(t, stderr)
		assert.Contains(t, errUtils.AllHints(err), "Use the identity flag or specify an identity as default.")
	})

	t.Run("undecodable component auth config", func(t *testing.T) {
		var err error
		stderr := captureStderr(t, func() {
			_, err = decodeAuthConfigFromStack(&schema.ConfigAndStacksInfo{
				ComponentAuthSection: schema.AtmosSectionMapType{"providers": 42},
			})
		})

		require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
		assert.Empty(t, stderr)
		assert.NotEmpty(t, errUtils.AllHints(err))
	})
}
