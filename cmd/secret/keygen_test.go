package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// configWithVaults builds an AtmosConfiguration whose secrets.providers contains the named vaults,
// each with the given kind.
func configWithVaults(vaults map[string]string) *schema.AtmosConfiguration {
	providersCfg := make(map[string]schema.SecretProviderConfig, len(vaults))
	for name, kind := range vaults {
		providersCfg[name] = schema.SecretProviderConfig{Kind: kind}
	}
	return &schema.AtmosConfiguration{
		Secrets: schema.SecretsConfig{Providers: providersCfg},
	}
}

func TestConfiguredVaults_SortedAndComplete(t *testing.T) {
	cfg := configWithVaults(map[string]string{
		"zeta":  "sops/age",
		"alpha": "sops/age",
		"mid":   "sops/gpg",
	})
	got := configuredVaults(cfg)
	require.Len(t, got, 3)
	// Sorted ascending for stable output.
	assert.Equal(t, "alpha", got[0])
	assert.Equal(t, "zeta", got[2])
}

func TestConfiguredVaults_Empty(t *testing.T) {
	assert.Empty(t, configuredVaults(&schema.AtmosConfiguration{}))
}

func TestVaultKind(t *testing.T) {
	cfg := configWithVaults(map[string]string{"dev-sops": "sops/age"})
	assert.Equal(t, "sops/age", vaultKind(cfg, "dev-sops"))
	// Unknown vault yields the zero value.
	assert.Empty(t, vaultKind(cfg, "missing"))
}

func TestTrackForKind(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{kind: "sops/age", want: "sops"},
		{kind: "ssl/x509", want: "ssl"},
		{kind: "sops", want: "sops"},
		{kind: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			assert.Equal(t, tt.want, trackForKind(tt.kind))
		})
	}
}

func TestVaultsHint(t *testing.T) {
	assert.Equal(t, "(none)", vaultsHint(nil))
	assert.Equal(t, "(none)", vaultsHint([]string{}))
	assert.Equal(t, "a, b, c", vaultsHint([]string{"a", "b", "c"}))
}

func TestResolveKeygenVault_ExplicitMatch(t *testing.T) {
	cfg := configWithVaults(map[string]string{"dev-sops": "sops/age", "prod-sops": "sops/age"})
	got, err := resolveKeygenVault(cfg, []string{"prod-sops"})
	require.NoError(t, err)
	assert.Equal(t, "prod-sops", got)
}

func TestResolveKeygenVault_ExplicitUnknown(t *testing.T) {
	cfg := configWithVaults(map[string]string{"dev-sops": "sops/age"})
	_, err := resolveKeygenVault(cfg, []string{"nope"})
	require.ErrorIs(t, err, ErrNoVault)
}

func TestResolveKeygenVault_SingleVault(t *testing.T) {
	cfg := configWithVaults(map[string]string{"only": "sops/age"})
	got, err := resolveKeygenVault(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, "only", got)
}

func TestResolveKeygenVault_NoVaults(t *testing.T) {
	_, err := resolveKeygenVault(&schema.AtmosConfiguration{}, nil)
	require.ErrorIs(t, err, ErrNoVault)
}

func TestResolveKeygenVault_Ambiguous(t *testing.T) {
	cfg := configWithVaults(map[string]string{"a": "sops/age", "b": "sops/age"})
	_, err := resolveKeygenVault(cfg, nil)
	require.ErrorIs(t, err, ErrAmbiguousVault)
}

func TestNotImplemented(t *testing.T) {
	// Smoke: notImplemented only logs to the UI channel; assert it does not panic.
	notImplemented("dev-sops", "sops/age")
}

func TestPrintKeygenResult(t *testing.T) {
	setupIO(t)
	res := &providers.KeygenResult{
		Vault:   "dev-sops",
		Kind:    "sops/age",
		Summary: "Generated an age key pair.",
		Outputs: []providers.KeygenOutput{
			{Label: "private identity", Location: "keys.txt", Sensitive: true},
			{Label: "public recipient", Location: ".sops.yaml", Sensitive: false},
		},
		Public: "age1xxxx",
	}
	// Smoke: renders without panicking; the data channel must be initialized for Public output.
	printKeygenResult(res)
}

func TestPrintKeygenResult_NoPublic(t *testing.T) {
	setupIO(t)
	res := &providers.KeygenResult{Vault: "dev-sops", Kind: "sops/age", Summary: "Done."}
	printKeygenResult(res)
}
