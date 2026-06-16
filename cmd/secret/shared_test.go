package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseScope_MissingComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// --stack present but --component absent → parseScope rejects on the component check (the
	// existing MissingScope tests omit both flags and stop at the stack check).
	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}
