package registryauth

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestDirectCredentialsCannotBeEmpty(t *testing.T) {
	_, err := fromCredentials(&types.OCICredentials{Registry: "registry.example.com"}, "registry.example.com")
	require.ErrorIs(t, err, errUtils.ErrIdentityCredentialsNone)
}
