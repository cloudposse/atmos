package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
)

func packageFixture(t *testing.T) (*manifest.Config, pluginpolicy.Option) {
	t.Helper()
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{
		Capability: "auth.provider", Path: []string{"auth", "providers"}, Version: 1,
		Validate: extension.ValidateAuth,
	}))
	definition := &manifest.Config{}
	definition.Metadata.Name, definition.Metadata.Version = "acme/auth", "1.0.0"
	definition.Spec.Runtime.Type = "native"
	definition.Spec.Capabilities = map[string]any{
		"auth": map[string]any{"providers": map[string]any{"oidc": map[string]any{}}},
	}
	return definition, pluginpolicy.WithExtensions(registry)
}

func governingPolicy(t *testing.T, predicate string) {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: corporate}\nspec:\n  policies:\n    plugins:\n      on: plugin\n      action: deny\n      rule: |\n        " + predicate + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
	restore, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(restore)
}

func TestRegistryAllowlistBlocksOtherRegistriesAndLocalDevelopment(t *testing.T) {
	governingPolicy(t, `input.registry != 'registry.example.com:5443'`)
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com:5443/team/auth"))
	require.NoError(t, pluginpolicy.Login(t.Context(), "registry.example.com:5443"))
	for _, source := range []string{
		"oci://registry.example.com/team/auth",
		"oci://registry.example.com.attacker.test/team/auth",
		"oci://public.example.com/team/auth",
		"",
	} {
		require.ErrorIs(t, pluginpolicy.Source(t.Context(), "install", source), errUtils.ErrPolicyDenied)
	}
	require.ErrorIs(t, pluginpolicy.Login(t.Context(), "public.example.com"), errUtils.ErrPolicyDenied)
}

func TestPackagePolicyUsesInspectedMetadata(t *testing.T) {
	governingPolicy(t, `input.phase == 'package' && (input.name != 'acme/auth' || input.runtime != 'native' || !('auth.provider' in input.capabilities))`)
	definition, descriptors := packageFixture(t)
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com/acme/auth"))
	require.NoError(t, pluginpolicy.Package(t.Context(), "install", "oci://registry.example.com/acme/auth", definition, descriptors, pluginpolicy.WithDigest("sha256:release")))
	definition.Metadata.Name = "other/auth"
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "activate", "oci://registry.example.com/acme/auth", definition, descriptors, pluginpolicy.WithDigest("sha256:release")), errUtils.ErrPolicyDenied)
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "activate", "", nil), errUtils.ErrPolicyInputInvalid)
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "activate", "", definition), errUtils.ErrPolicyInputUnavailable)
}

func TestPluginSourcesAreCanonicalAndCredentialFree(t *testing.T) {
	governingPolicy(t, `input.source == 'oci://registry.example.com/acme/auth' && input.repository == 'acme/auth' && input.phase == 'source' && input.name == '' && !input.development`)
	for _, source := range []string{"registry.example.com/acme/auth", "oci://registry.example.com/acme/auth:v1.0.0"} {
		require.ErrorIs(t, pluginpolicy.Source(t.Context(), "install", source), errUtils.ErrPolicyDenied)
	}
	for _, source := range []string{"oci://user:secret@registry.example.com/acme/auth", "oci://registry.example.com/acme/auth?token=secret"} {
		err := pluginpolicy.Source(t.Context(), "install", source)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		require.NotContains(t, err.Error(), "secret")
		err = pluginpolicy.Package(t.Context(), "install", source, &manifest.Config{})
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		require.NotContains(t, err.Error(), "secret")
	}
	require.ErrorIs(t, pluginpolicy.Login(t.Context(), "user:secret@registry.example.com"), errUtils.ErrPolicyInputInvalid)
}

func TestLocalPolicyExemptionIsExplicit(t *testing.T) {
	governingPolicy(t, `!input.development && input.registry != 'registry.example.com'`)
	require.NoError(t, pluginpolicy.Source(t.Context(), "test", ""))
	definition, descriptors := packageFixture(t)
	require.NoError(t, pluginpolicy.Package(t.Context(), "test", "", definition, descriptors))
	require.ErrorIs(t, pluginpolicy.Source(t.Context(), "test", "oci://other.example.com/acme/auth"), errUtils.ErrPolicyDenied)
}

func TestArtifactOriginCannotBeHiddenBehindAnApprovedRepository(t *testing.T) {
	governingPolicy(t, `input.origin in ['development', 'layout'] || input.registry != 'registry.example.com'`)
	const source = "oci://registry.example.com/acme/auth"
	require.NoError(t, pluginpolicy.Source(t.Context(), "activate", source, pluginpolicy.WithCache()))
	require.ErrorIs(t, pluginpolicy.Source(t.Context(), "install", source, pluginpolicy.WithLayout()), errUtils.ErrPolicyDenied)
	definition, descriptors := packageFixture(t)
	require.NoError(t, pluginpolicy.Package(t.Context(), "activate", source, definition, descriptors, pluginpolicy.WithCache()))
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "install", source, definition, descriptors, pluginpolicy.WithLayout()), errUtils.ErrPolicyDenied)
}

func TestCapabilityPoliciesDiscoverFutureExtensionsWithoutExecutingValidators(t *testing.T) {
	governingPolicy(t, `input.capabilities == ['example.audit']`)
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{
		Capability: "example.audit", Path: []string{"reviews"}, Version: 1,
		Validate: func(extension.Binding) error {
			t.Fatal("policy capability collection must not execute extension validators")
			return nil
		},
	}))
	definition := &manifest.Config{}
	definition.Spec.Capabilities = map[string]any{
		"reviews": map[string]any{"one": map[string]any{}, "two": map[string]any{}},
	}
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "install", "", definition,
		pluginpolicy.WithExtensions(registry)), errUtils.ErrPolicyDenied, "normalized capabilities must be deduplicated")
	definition.Spec.Capabilities["unknown"] = true
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "install", "", definition,
		pluginpolicy.WithExtensions(registry)), errUtils.ErrPluginUnsupportedExtension)
}

func TestMissingPolicyIsNoOpButEmptyPolicyFailsClosed(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("TEST_GIT_ROOT", "")
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", ""))
	require.NoError(t, pluginpolicy.Package(t.Context(), "install", "", nil))
	require.NoError(t, pluginpolicy.Login(t.Context(), "registry.example.com"))
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.ErrorIs(t, pluginpolicy.Source(t.Context(), "install", ""), errUtils.ErrNoPoliciesConfigured)
	require.ErrorIs(t, pluginpolicy.Package(t.Context(), "install", "", nil), errUtils.ErrNoPoliciesConfigured)
	require.ErrorIs(t, pluginpolicy.Login(t.Context(), "registry.example.com"), errUtils.ErrNoPoliciesConfigured)
}
