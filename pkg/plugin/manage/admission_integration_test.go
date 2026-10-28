package manage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

func managementAdmissionPolicy(t *testing.T, rules ...string) {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	rule := "input.registry != 'corporate.example.com'"
	if len(rules) > 0 {
		rule = rules[0]
	}
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: corporate}\nspec:\n  policies:\n    plugins:\n      on: plugin\n      action: deny\n      rule: " + rule + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
	restore, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(restore)
}

func TestAdmissionManagementDenialsPreserveProjectFiles(t *testing.T) {
	for _, operation := range []string{"install", "registry add"} {
		t.Run(operation, func(t *testing.T) {
			manager, source, _ := setup(t)
			path := filepath.Join(manager.Project, manager.ConfigFile)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			managementAdmissionPolicy(t)

			if operation == "install" {
				err = manager.Add(t.Context(), manage.AddOptions{Source: source, Alias: "fixture"})
			} else {
				err = manager.EditRegistry(t.Context(), "external", &schema.PluginRegistry{Catalog: "oci://public.example.com/plugins/catalog:stable"})
			}

			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			_, err = os.Stat(pluginlock.Path(manager.Project))
			require.ErrorIs(t, err, os.ErrNotExist)
			require.Empty(t, manager.Config.Plugins.Packages)
			require.Empty(t, manager.Config.Plugins.Registries)
		})
	}
}

func TestAdmissionApprovedRegistryCanBeConfigured(t *testing.T) {
	manager, _, _ := setup(t)
	managementAdmissionPolicy(t)
	registry := schema.PluginRegistry{Catalog: "oci://corporate.example.com/plugins/catalog:stable"}

	require.NoError(t, manager.EditRegistry(t.Context(), "corporate", &registry))

	reloadConfiguration(t, manager)
	require.Equal(t, registry, manager.Config.Plugins.Registries["corporate"])
	raw, err := os.ReadFile(filepath.Join(manager.Project, manager.ConfigFile))
	require.NoError(t, err)
	require.Contains(t, string(raw), "# keep this comment")
}

func TestAdmissionRegistryDiscoveryChecksEverySource(t *testing.T) {
	for _, registry := range []schema.PluginRegistry{
		{Host: "public.example.com"},
		{Repositories: []string{"oci://public.example.com/acme/one"}},
	} {
		t.Run("denied registry", func(t *testing.T) {
			manager, _, _ := setup(t)
			path := filepath.Join(manager.Project, manager.ConfigFile)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			managementAdmissionPolicy(t)
			err = manager.EditRegistry(t.Context(), "external", &registry)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestAdmissionRegistryChecksRepositoryAfterAllowedCatalog(t *testing.T) {
	manager, _, _ := setup(t)
	path := filepath.Join(manager.Project, manager.ConfigFile)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	managementAdmissionPolicy(t, "input.repository == 'acme/blocked'")
	registry := schema.PluginRegistry{
		Catalog:      "oci://corporate.example.com/acme/catalog:stable",
		Repositories: []string{"oci://corporate.example.com/acme/allowed", "oci://corporate.example.com/acme/blocked"},
	}
	err = manager.EditRegistry(t.Context(), "corporate", &registry)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
