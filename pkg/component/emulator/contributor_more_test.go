package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	emutarget "github.com/cloudposse/atmos/pkg/emulator/target"
	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/schema"
)

// emulatorBoundGenCtx builds a GeneratorContext whose component is bound to an
// AWS emulator identity, so emulatorBinding resolves to the aws Terraform provider.
func emulatorBoundGenCtx() *generator.GeneratorContext {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Auth.Identities = map[string]schema.Identity{
		"local-aws": {Kind: types.IdentityKindAWSEmulator, Emulator: "local/aws"},
	}
	return &generator.GeneratorContext{
		AtmosConfig: atmosConfig,
		StackInfo:   &schema.ConfigAndStacksInfo{Identity: "local-aws"},
		Stack:       "dev",
	}
}

func TestContribute_EmulatorBoundReturnsProviderFragment(t *testing.T) {
	providerName, ok := emutarget.TerraformProviderName("aws")
	require.True(t, ok, "aws target must have a Terraform provider name")

	fragment := map[string]any{"region": "us-east-1", "s3_use_path_style": true}
	mgr := &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resProfile: emu.Profile{Provider: fragment},
	}
	stubPrepare(t, validSection(), nil, mgr)

	got, err := providerContributor{}.Contribute(context.Background(), emulatorBoundGenCtx())
	require.NoError(t, err)
	assert.Equal(t, map[string]any{providerName: fragment}, got)
	assert.Equal(t, 1, mgr.resCalls)
	assert.Equal(t, "local", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestContribute_EmptyProviderContributesNothing(t *testing.T) {
	mgr := &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resProfile: emu.Profile{Provider: nil},
	}
	stubPrepare(t, validSection(), nil, mgr)

	got, err := providerContributor{}.Contribute(context.Background(), emulatorBoundGenCtx())
	require.NoError(t, err)
	assert.Nil(t, got, "a target with no provider fragment contributes nothing")
}

func TestContribute_PrepareError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	orig := initCliConfig
	t.Cleanup(func() { initCliConfig = orig })
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errBoom
	}
	_, err := providerContributor{}.Contribute(context.Background(), emulatorBoundGenCtx())
	require.ErrorIs(t, err, errBoom)
}

func TestContribute_ResolveError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{
		psStatuses: []emu.Status{{Name: "aws", Stack: "local", Status: "running"}},
		resErr:     errBoom,
	})
	_, err := providerContributor{}.Contribute(context.Background(), emulatorBoundGenCtx())
	require.ErrorIs(t, err, errBoom)
}

func TestContributorName(t *testing.T) {
	assert.Equal(t, "emulator", providerContributor{}.Name())
}
