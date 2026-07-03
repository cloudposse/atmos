package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: an emulator-identity-kind rename must fail the build here.
var _ = []string{
	types.IdentityKindAWSEmulator,
	types.IdentityKindGCPEmulator,
	types.IdentityKindAzureEmulator,
	types.IdentityKindKubernetesEmulator,
}

func TestSelectedIdentity(t *testing.T) {
	defaultAuth := &schema.AuthConfig{Identities: map[string]schema.Identity{
		"local-aws": {Kind: types.IdentityKindAWSEmulator, Emulator: "aws", Default: true},
		"other":     {Kind: "aws/permission-set"},
	}}
	noDefaultAuth := &schema.AuthConfig{Identities: map[string]schema.Identity{
		"other": {Kind: "aws/permission-set"},
	}}

	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		auth *schema.AuthConfig
		want string
	}{
		{
			name: "flag wins over default",
			info: &schema.ConfigAndStacksInfo{Identity: "other"},
			auth: defaultAuth,
			want: "other",
		},
		{
			name: "select sentinel ignored, falls back to default identity",
			info: &schema.ConfigAndStacksInfo{Identity: cfg.IdentityFlagSelectValue},
			auth: defaultAuth,
			want: "local-aws",
		},
		{
			name: "no flag, default identity",
			info: &schema.ConfigAndStacksInfo{},
			auth: defaultAuth,
			want: "local-aws",
		},
		{
			name: "no flag, no default",
			info: &schema.ConfigAndStacksInfo{},
			auth: noDefaultAuth,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, selectedIdentity(tt.info, tt.auth))
		})
	}
}

func TestContribute_NotEmulatorBound(t *testing.T) {
	c := providerContributor{}

	// No auth config / no identity → no contribution, no error.
	got, err := c.Contribute(context.Background(), &generator.GeneratorContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		StackInfo:   &schema.ConfigAndStacksInfo{Identity: "local-aws"},
		Stack:       "dev",
	})
	require.NoError(t, err)
	assert.Nil(t, got, "unknown identity contributes nothing")

	// Identity exists but is not emulator-bound → no contribution.
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Auth.Identities = map[string]schema.Identity{"cloud": {Kind: "aws/permission-set"}}
	got, err = c.Contribute(context.Background(), &generator.GeneratorContext{
		AtmosConfig: atmosConfig,
		StackInfo:   &schema.ConfigAndStacksInfo{Identity: "cloud"},
		Stack:       "dev",
	})
	require.NoError(t, err)
	assert.Nil(t, got, "non-emulator identity contributes nothing")
}

func TestContribute_KubernetesContributesNothing(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Auth.Identities = map[string]schema.Identity{
		"local-k8s": {Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"},
	}
	got, err := providerContributor{}.Contribute(context.Background(), &generator.GeneratorContext{
		AtmosConfig: atmosConfig,
		StackInfo:   &schema.ConfigAndStacksInfo{Identity: "local-k8s"},
		Stack:       "dev",
	})
	require.NoError(t, err)
	assert.Nil(t, got, "kubernetes target has no Terraform provider fragment")
}

func TestContributorRegisteredWithGenerator(t *testing.T) {
	names := make([]string, 0)
	for _, c := range generator.ProviderContributors() {
		names = append(names, c.Name())
	}
	assert.Contains(t, names, contributorName, "emulator contributor registered at init")
}
