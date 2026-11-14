package signature

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	sigcore "github.com/cloudposse/atmos/pkg/signature"
)

func TestPluginErrorKeepsBothLegacyClassificationsForOfflineTrust(t *testing.T) {
	cause := errors.New("trusted root cache is empty")
	err := pluginError(&sigcore.Error{Kind: errUtils.ErrSignatureOffline, Cause: cause})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid, "offline trust failures are still signature failures")
	require.ErrorIs(t, err, cause)

	err = pluginError(&sigcore.Error{Kind: errUtils.ErrSignatureOffline})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)

	err = pluginError(&sigcore.Error{Kind: errUtils.ErrSignatureConfigInvalid})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NotErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)

	// Production shape: the trust loader failure is wrapped as invalid, with offline in the cause chain.
	err = pluginError(&sigcore.Error{Kind: errUtils.ErrSignatureInvalid, Cause: errUtils.ErrSignatureOffline})
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)

	err = pluginError(&sigcore.Error{Kind: errUtils.ErrSignatureInvalid, Cause: errors.New("bad signature")})
	require.NotErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
}
