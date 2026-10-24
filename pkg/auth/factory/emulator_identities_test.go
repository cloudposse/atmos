package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a kind-constant rename must fail the build here too.
var _ = []string{
	types.IdentityKindAWSEmulator,
	types.IdentityKindGCPEmulator,
	types.IdentityKindAzureEmulator,
	types.IdentityKindKubernetesEmulator,
}

func TestNewIdentity_EmulatorKindsRegistered(t *testing.T) {
	for _, kind := range types.EmulatorIdentityKinds {
		t.Run(kind, func(t *testing.T) {
			require.True(t, defaultFactory.HasIdentity(kind), "kind %q registered with the default factory", kind)

			config := &schema.Identity{Kind: kind, Emulator: "emu"}
			id, err := NewIdentity("local-"+kind, config)
			require.NoError(t, err)

			// SetConfig must have run via the ConfigSetter path so Kind() resolves.
			assert.Equal(t, kind, id.Kind(), "config injected via ConfigSetter")
			require.NoError(t, id.Validate(), "fully-formed emulator identity validates")
		})
	}
}

func TestNewIdentity_EmulatorRequiresEmulatorRef(t *testing.T) {
	id, err := NewIdentity("bad", &schema.Identity{Kind: types.IdentityKindAWSEmulator})
	require.NoError(t, err, "construction succeeds")
	require.Error(t, id.Validate(), "validation fails without emulator reference")
}
