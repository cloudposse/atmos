package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

func installAdmissionPolicy(t *testing.T, predicate string) {
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

func TestAdmissionExplicitInstallDeniesSourceBeforeRegistryOrCache(t *testing.T) {
	service, _, _ := installFixture(t)
	service.Registry = NewMockRegistry(gomock.NewController(t))
	installAdmissionPolicy(t, `input.registry != 'corporate.example.com'`)

	_, err := service.Prepare(t.Context(), &Request{Source: "oci://public.example.com/acme/fixture", Constraint: "*"})

	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	entries, err := os.ReadDir(service.Cache.Directory)
	require.NoError(t, err)
	require.Empty(t, entries, "a denied source must not create a release or extraction lock")
}

func TestAdmissionFreshInstallUsesInspectedPackageName(t *testing.T) {
	service, release, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	installAdmissionPolicy(t, `input.phase == 'package' && input.name != 'corporate/approved'`)

	_, err := service.Prepare(t.Context(), &Request{Source: "oci://corporate.example.com/corporate/approved", Constraint: "*", Layout: layout})

	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Equal(t, "acme/fixture", release.Definition.Metadata.Name)
	entries, err := os.ReadDir(service.Cache.Directory)
	require.NoError(t, err)
	require.Empty(t, entries, "metadata admission must happen before payload extraction")
}

func TestAdmissionCachedInstallRechecksCurrentPolicy(t *testing.T) {
	service, _, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	request := &Request{Source: "oci://corporate.example.com/acme/fixture", Constraint: "*", Layout: layout}
	entry, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	request.Layout, request.Locked = "", entry
	service.Config.Plugins.Policy.Offline = true
	service.Registry = NewMockRegistry(gomock.NewController(t))
	installAdmissionPolicy(t, `input.phase == 'package' && input.name != 'corporate/approved'`)

	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)

	forged := *entry
	forged.Plugin = "corporate/approved"
	require.ErrorIs(t, service.Cache.CheckPolicy(t.Context(), "activate", &forged, service.Platform, service.Descriptors), errUtils.ErrPluginFrozenDrift)
	request.Locked = &forged
	request.Layout = layout
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginFrozenDrift,
		"a forged lock name must fail integrity checks instead of satisfying the name policy")
	require.Equal(t, "acme/fixture", entry.Plugin)
}

func TestAdmissionLayoutOriginDeniedBeforeReadingFiles(t *testing.T) {
	service, _, _ := installFixture(t)
	service.Registry = NewMockRegistry(gomock.NewController(t))
	installAdmissionPolicy(t, `input.origin == 'layout'`)

	_, err := service.Prepare(t.Context(), &Request{
		Source: "oci://corporate.example.com/acme/fixture", Constraint: "*",
		Layout: filepath.Join(t.TempDir(), "nonexistent"),
	})

	require.ErrorIs(t, err, errUtils.ErrPolicyDenied, "local input must be admitted before trying to open its layout")
	entries, err := os.ReadDir(service.Cache.Directory)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestAdmissionAllowDoesNotBypassRequiredSignature(t *testing.T) {
	service, _, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	installAdmissionPolicy(t, "false")
	service.Config.Plugins.Policy = schema.PluginPolicy{
		Offline: true, RequireSignature: true,
		TrustedPublishers: []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}},
	}

	_, err := service.Prepare(t.Context(), &Request{Source: "oci://corporate.example.com/acme/fixture", Constraint: "*", Layout: layout})

	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	entries, err := os.ReadDir(service.Cache.Directory)
	require.NoError(t, err)
	require.Empty(t, entries)
}
