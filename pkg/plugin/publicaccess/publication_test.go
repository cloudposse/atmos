package publicaccess

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPublicationError(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	err := PublicationError("oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets", digest, errUtils.ErrOCIIntegrity)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	require.ErrorContains(t, err, "was published")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "https://github.com/orgs/cloudposse/packages/container/package/atmos%2Fplugins%2Fauth-secrets/settings")
	require.Contains(t, hints, "Change visibility → Public")
	require.Contains(t, hints, "already public")
	require.Contains(t, hints, "atmos plugin verify-public oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets --reference "+digest)
	require.Contains(t, hints, "rerun the same")

	err = PublicationError("registry.example.com/team/plugin", digest, errUtils.ErrOCIRegistry)
	require.NotContains(t, strings.Join(errUtils.AllHints(err), "\n"), "github.com")
}
