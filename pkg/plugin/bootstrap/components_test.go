package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

const componentManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: fixture/components
  version: 1.0.0
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    PLATFORM: {executable: bin/plugin}
  capabilities:
    components:
      discovery:
        fixture/hcl: {component_type: terraform, schema: schemas/config.json}
      state:
        fixture/hcl: {component_type: terraform, schema: schemas/config.json}
`

func localComponentFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	directory := localPackageFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "config.json"), []byte(`{"type":"object"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(componentManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	return &schema.AtmosConfiguration{BasePathConfigDir: t.TempDir(), Plugins: schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"fixture": {Path: directory}}}}
}

func componentReferences() []ComponentReference {
	return []ComponentReference{{Provider: "fixture/hcl", Capability: extension.ComponentDiscovery}, {Provider: "fixture/hcl", Capability: extension.ComponentState}}
}

func TestLoadComponentsIsExplicitLazyAndKeepsPrivateRegistries(t *testing.T) {
	previous := component.ListProviders()
	t.Cleanup(func() {
		component.Reset()
		for _, providers := range previous {
			for _, provider := range providers {
				require.NoError(t, component.Register(provider))
			}
		}
	})
	engine := terraform.New(nil)
	require.NoError(t, component.Register(engine))
	atmos := localComponentFixture(t)
	registry, err := LoadComponents(t.Context(), atmos, componentReferences())
	require.NoError(t, err, "static non-executable fixture must not run during activation")
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	p, ok := registry.Get("fixture/hcl")
	require.True(t, ok)
	require.Equal(t, "terraform", p.ComponentType())
	require.True(t, p.Supports(extension.ComponentState))
	requireNoLock(t, atmos)
	second, err := LoadComponents(t.Context(), atmos, componentReferences())
	require.NoError(t, err)
	p2, _ := second.Get("fixture/hcl")
	require.NotSame(t, p, p2, "one invocation must not overwrite another provider registry")
	_, ok = registry.Get("terraform")
	require.False(t, ok)
	after, found := component.GetProvider("terraform")
	require.True(t, found)
	require.Same(t, engine, after, "configuration activation must not replace the execution engine")
}

func TestNoComponentReferencesDoNotInspectBrokenPackages(t *testing.T) {
	t.Parallel()
	atmos := localComponentFixture(t)
	require.NoError(t, os.Remove(filepath.Join(atmos.Plugins.Packages["fixture"].Path, "plugin.yaml")))
	registry, err := LoadComponents(t.Context(), atmos, nil)
	require.NoError(t, err)
	require.Empty(t, registry.Names())
	requireNoLock(t, atmos)
	_, err = LoadComponents(t.Context(), atmos, componentReferences())
	require.Error(t, err, "an explicit reference must report a broken enabled package")
}

func TestDisabledAndMissingCapabilitiesCannotActivate(t *testing.T) {
	t.Parallel()
	atmos := localComponentFixture(t)
	disabled := false
	entry := atmos.Plugins.Packages["fixture"]
	entry.Enabled = &disabled
	atmos.Plugins.Packages["fixture"] = entry
	_, err := LoadComponents(t.Context(), atmos, componentReferences())
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	entry.Enabled = nil
	atmos.Plugins.Packages["fixture"] = entry
	refs := append(componentReferences(), ComponentReference{Provider: "fixture/hcl", Capability: extension.ComponentPreparation})
	_, err = LoadComponents(t.Context(), atmos, refs)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	_, err = LoadComponents(t.Context(), nil, refs)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = LoadComponents(t.Context(), atmos, []ComponentReference{{Provider: "fixture/hcl", Capability: "auth.provider"}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = LoadComponents(ctx, atmos, refs)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDuplicateComponentOwnersFailBeforeRegistration(t *testing.T) {
	t.Parallel()
	atmos := localComponentFixture(t)
	atmos.Plugins.Packages["duplicate"] = atmos.Plugins.Packages["fixture"]
	registry, err := LoadComponents(t.Context(), atmos, componentReferences())
	require.ErrorIs(t, err, errUtils.ErrDuplicatePluginExtension)
	require.Nil(t, registry)
	requireNoLock(t, atmos)
}

func TestLockedComponentReferencesAreCapabilityQualified(t *testing.T) {
	t.Parallel()
	file := &pluginlock.File{}
	file.Spec.Packages = map[string]pluginlock.Entry{"fixture": {Exports: []pluginlock.Export{{Capability: "auth.provider", Name: "fixture/hcl"}, {Capability: extension.ComponentState, Name: "fixture/hcl"}}}}
	exports, known, err := componentPackageExports("fixture", &config.Selections{}, file)
	require.NoError(t, err)
	require.True(t, known)
	require.False(t, exports[ComponentReference{Provider: "fixture/hcl", Capability: extension.ComponentDiscovery}])
	require.True(t, exports[ComponentReference{Provider: "fixture/hcl", Capability: extension.ComponentState}])
	_, known, err = componentPackageExports("missing", &config.Selections{}, file)
	require.NoError(t, err)
	require.False(t, known)
}

func TestLoadComponentsRestoresReleasedPackageAndReusesLock(t *testing.T) {
	atmos, requests := automaticFixture(t, componentManifest)
	registry, err := LoadComponents(t.Context(), atmos, componentReferences())
	require.NoError(t, err)
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	require.Positive(t, requests.Load())
	before, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	requests.Store(0)
	registry, err = LoadComponents(t.Context(), atmos, componentReferences())
	require.NoError(t, err)
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	require.Zero(t, requests.Load(), "cached providers must not contact the registry")
	after, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	require.Equal(t, before, after)
	atmos.Plugins.CacheDir = t.TempDir()
	_, err = LoadComponents(t.Context(), atmos, componentReferences())
	require.NoError(t, err)
	require.Positive(t, requests.Load(), "missing cache is restored from the reviewed lock")
}

func TestUnreferencedAndUnlockedComponentPackagesDoNotRunInCI(t *testing.T) {
	withCI(t, true)
	atmos, requests := automaticFixture(t, componentManifest)
	registry, err := LoadComponents(t.Context(), atmos, nil)
	require.NoError(t, err)
	require.Empty(t, registry.Names())
	require.Zero(t, requests.Load())
	_, err = LoadComponents(t.Context(), atmos, componentReferences())
	require.ErrorIs(t, err, errUtils.ErrPluginUnlockedInCI)
	require.Zero(t, requests.Load())
	requireNoLock(t, atmos)
}

func TestMalformedComponentMetadataCannotActivate(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*testing.T, string){
		"unknown capability": func(t *testing.T, dir string) {
			raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(strings.ReplaceAll(string(raw), "discovery:", "unknown:")), 0o600))
		},
		"missing executable": func(t *testing.T, dir string) { require.NoError(t, os.Remove(filepath.Join(dir, "bin", "plugin"))) },
		"missing schema": func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "schemas", "config.json")))
		},
		"inconsistent type": func(t *testing.T, dir string) {
			raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(strings.Replace(string(raw), "component_type: terraform", "component_type: other", 1)), 0o600))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			atmos := localComponentFixture(t)
			edit(t, atmos.Plugins.Packages["fixture"].Path)
			registry, err := LoadComponents(t.Context(), atmos, componentReferences())
			require.Error(t, err)
			require.Nil(t, registry)
		})
	}
}

const optionsManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: fixture/options
  version: 1.0.0
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    PLATFORM: {executable: bin/plugin}
  capabilities:
    components:
      discovery:
        tool:
          component_type: terraform
          schema: schemas/config.json
          modes: [native, executable]
          options:
            - {name: level, type: string, description: Level., commands: [plan, apply], modes: [executable]}
            - {name: always, type: bool, description: Always., commands: [plan]}
`

func optionsFixture(t *testing.T, mode, manifest string) *schema.AtmosConfiguration {
	t.Helper()
	directory := localPackageFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "config.json"), []byte(`{"type":"object"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(manifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	atmos := &schema.AtmosConfiguration{BasePathConfigDir: t.TempDir(), Plugins: schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"fixture": {Path: directory}}}}
	atmos.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"tool": {Enabled: true, Mode: mode}}
	return atmos
}

func TestComponentOptionsReadStaticMetadataWithoutStartingThePlugin(t *testing.T) {
	for mode, want := range map[string][]string{"": {"always"}, "native": {"always"}, "executable": {"level", "always"}} {
		providers, err := ComponentOptions(t.Context(), optionsFixture(t, mode, optionsManifest))
		require.NoError(t, err, "the fixture executable does not exist, so nothing may start")
		require.Len(t, providers, 1)
		require.Equal(t, "tool", providers[0].Section)
		names := []string{}
		for index := range providers[0].Options {
			if providers[0].Options[index].AppliesTo(providers[0].Mode) {
				names = append(names, providers[0].Options[index].Name)
			}
		}
		require.ElementsMatch(t, want, names, mode)
	}
}

func TestComponentOptionsIgnoreDisabledAndUnknownProviders(t *testing.T) {
	atmos := optionsFixture(t, "", optionsManifest)
	atmos.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"tool": {Enabled: false}, "other": {Enabled: true}}
	providers, err := ComponentOptions(t.Context(), atmos)
	require.NoError(t, err)
	require.Empty(t, providers)
	atmos.Components.Terraform.ConfigurationProviders = nil
	providers, err = ComponentOptions(t.Context(), atmos)
	require.NoError(t, err)
	require.Nil(t, providers)
}

func TestRegisterComponentFlagsAddsFlagsAndReportsDeclarationErrors(t *testing.T) {
	root := &cobra.Command{Use: "terraform"}
	plan := &cobra.Command{Use: "plan"}
	root.AddCommand(plan, &cobra.Command{Use: "apply"})
	require.NoError(t, RegisterComponentFlags(t.Context(), optionsFixture(t, "executable", optionsManifest), root))
	require.NotNil(t, plan.Flags().Lookup("tool-level"))
	require.NotNil(t, plan.Flags().Lookup("tool-always"))

	broken := strings.ReplaceAll(optionsManifest, "type: string, description: Level.", "type: float, description: Level.")
	err := RegisterComponentFlags(t.Context(), optionsFixture(t, "executable", broken), &cobra.Command{Use: "terraform"})
	require.Error(t, err, "a declaration problem is reported")

	// A package that cannot be read does not break unrelated commands; activation reports it later.
	missing := optionsFixture(t, "", optionsManifest)
	missing.Plugins.Packages["fixture"] = schema.PluginPackage{Path: filepath.Join(t.TempDir(), "absent")}
	require.NoError(t, RegisterComponentFlags(t.Context(), missing, &cobra.Command{Use: "terraform"}))
}

func TestComponentOptionsSkipPackagesThatAreNotInstalled(t *testing.T) {
	atmos := optionsFixture(t, "executable", optionsManifest)
	// A registry package with no lock entry has nothing on disk to read, and reading never installs it.
	atmos.Plugins.Packages["remote"] = schema.PluginPackage{Source: "oci://registry.example.test/atmos/remote", Version: "1.0.0"}
	providers, err := ComponentOptions(t.Context(), atmos)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, "tool", providers[0].Name)
}

func TestComponentOptionsSurviveAnUnrelatedPackageThatCannotBeRead(t *testing.T) {
	atmos := optionsFixture(t, "executable", optionsManifest)
	unrelated := localPackageFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(unrelated, "plugin.yaml"), []byte("apiVersion: atmos/v1\nkind: [unterminated\n"), 0o600))
	atmos.Plugins.Packages["future"] = schema.PluginPackage{Path: unrelated}

	providers, err := ComponentOptions(t.Context(), atmos)
	require.NoError(t, err)
	require.Len(t, providers, 1, "the healthy provider keeps its options")
	require.Equal(t, "tool", providers[0].Name)
	require.Equal(t, "level", providers[0].Options[0].Name)
}
