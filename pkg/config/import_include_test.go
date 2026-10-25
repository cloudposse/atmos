package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{}}

func TestMergeConfig_ImportsWithDeferredInclude(t *testing.T) {
	for _, env := range []string{"!include env.yaml", "\"!include env.yaml\"", "{UPPER: value}"} {
		t.Run(env, func(t *testing.T) {
			setupTestAdapters()
			t.Cleanup(ResetImportAdapterRegistry)
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("TEST_GIT_ROOT", dir)
			createConfigFile(t, dir, "env.yaml", "UPPER: value\n")
			createConfigFile(t, dir, "base.yaml", `mcp:
  servers:
    inherited:
      command: imported-control
auth:
  identities:
    security-control:
      kind: aws/permission-set
      via: {provider: missing-control}
`)
			createConfigFile(t, dir, "atmos.yaml", `base_path: ./
import: [base.yaml]
mcp:
  servers:
    local:
      command: local-control
      env: `+env+"\n")
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, mergeConfig(v, dir, CliConfigFileName, true))
			assert.Equal(t, "imported-control", v.GetString("mcp.servers.inherited.command"))
			assert.Equal(t, "missing-control", v.GetString("auth.identities.security-control.via.provider"))
			assert.Equal(t, "local-control", v.GetString("mcp.servers.local.command"))
		})
	}
}

func TestMergeConfig_RequiredImportsWithDeferredInclude(t *testing.T) {
	for _, test := range []struct {
		name      string
		imports   string
		wantError error
	}{
		{"required", "[{path: missing.yaml, skip_if_missing: false}]", errUtils.ErrRequiredImportFailed},
		{"optional", "[missing.yaml]", nil},
		{"invalid condition", "[{path: missing.yaml, when: '!cel ((('}]", errUtils.ErrInvalidConfigImport},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupTestAdapters()
			t.Cleanup(ResetImportAdapterRegistry)
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("TEST_GIT_ROOT", dir)
			createConfigFile(t, dir, "env.yaml", "UPPER: value\n")
			createConfigFile(t, dir, "atmos.yaml", "base_path: ./\nimport: "+test.imports+`
mcp:
  servers:
    local:
      command: local-control
      env: !include env.yaml
`)
			v := viper.New()
			v.SetConfigType("yaml")
			err := mergeConfig(v, dir, CliConfigFileName, true)
			if test.wantError != nil {
				require.ErrorIs(t, err, test.wantError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "local-control", v.GetString("mcp.servers.local.command"))
			}
		})
	}
}

// importContextAdapter captures the settings actually delivered to an import
// resolver without contacting a remote endpoint.
type importContextAdapter struct{ config *schema.AtmosConfiguration }

func (a *importContextAdapter) Schemes() []string { return nil }
func (a *importContextAdapter) Resolve(_ context.Context, _, _, _ string, _, _ int, cfg *schema.AtmosConfiguration) ([]ResolvedPaths, error) {
	a.config = cfg
	return nil, nil
}

func TestMergeImports_PreservesResolverContext(t *testing.T) {
	ResetImportAdapterRegistry()
	t.Cleanup(ResetImportAdapterRegistry)
	adapter := &importContextAdapter{}
	SetDefaultAdapter(adapter)
	dir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	v := viper.New()
	v.Set("base_path", dir)
	v.Set("import", []string{"remote.yaml"})
	v.Set("imports.ttl", "1ms")
	v.Set("stacks.base_path", "custom-stacks")
	v.Set("settings.github_username", "fixture-reader")
	v.Set("settings.pro.enabled", true)
	v.Set("auth.providers.test.kind", "aws/iam-identity-center")
	// An unrelated field is deliberately still a scalar before function evaluation.
	v.Set("mcp.servers.local.env", "!include env.yaml")
	_, err := mergeImports(v, dir, "", "")
	require.NoError(t, err)
	require.NotNil(t, adapter.config)
	assert.Equal(t, dir, adapter.config.BasePath)
	assert.Equal(t, "1ms", adapter.config.Imports.TTL)
	assert.Equal(t, "custom-stacks", adapter.config.Stacks.BasePath)
	assert.Equal(t, "fixture-reader", adapter.config.Settings.GithubUsername)
	assert.True(t, adapter.config.Settings.Pro.Enabled)
	assert.Equal(t, "aws/iam-identity-center", adapter.config.Auth.Providers["test"].Kind)
}

func TestMergeConfig_DeferredImportContextSections(t *testing.T) {
	for _, test := range []struct{ name, config, key, want string }{
		{"auth", "auth: !include section.yaml", "auth.providers.local.kind", "mock"},
		{"nested auth", "auth:\n  providers: !include section.yaml .providers", "auth.providers.local.kind", "mock"},
		{"settings", "settings: !include section.yaml", "settings.github_username", "fixture-reader"},
		{"imports", "imports: !include section.yaml", "imports.ttl", "1h"},
	} {
		for _, imports := range []string{"", "import: [base.yaml]\n"} {
			t.Run(test.name+"/"+imports, func(t *testing.T) {
				setupTestAdapters()
				t.Cleanup(ResetImportAdapterRegistry)
				dir := t.TempDir()
				t.Chdir(dir)
				t.Setenv("TEST_GIT_ROOT", dir)
				createConfigFile(t, dir, "base.yaml", "mcp:\n  servers:\n    inherited:\n      command: imported-control\n")
				createConfigFile(t, dir, "section.yaml", "providers:\n  local:\n    kind: mock\ngithub_username: fixture-reader\nttl: 1h\n")
				createConfigFile(t, dir, "atmos.yaml", "base_path: ./\n"+imports+test.config+"\n")
				v := viper.New()
				v.SetConfigType("yaml")
				require.NoError(t, mergeConfig(v, dir, CliConfigFileName, true))
				assert.Equal(t, test.want, v.GetString(test.key))
				if imports != "" {
					assert.Equal(t, "imported-control", v.GetString("mcp.servers.inherited.command"))
				}
			})
		}
	}
}

func TestDecodeConfigForImports_RejectsStaticInvalidSections(t *testing.T) {
	for _, section := range []string{"auth", "settings", "imports"} {
		t.Run(section, func(t *testing.T) {
			v := viper.New()
			v.Set(section, "invalid-static-value")
			_, err := decodeConfigForImports(v)
			require.ErrorIs(t, err, errUtils.ErrMergeConfiguration)
		})
	}
}

func TestDeferredProvidersLoadProvisionedIdentities(t *testing.T) {
	for _, auth := range []string{
		"auth:\n  providers: !include providers.yaml\n  identities:\n    developer:\n      default: true\n",
		"auth: !include auth.yaml\n",
	} {
		t.Run(auth, func(t *testing.T) {
			setupTestAdapters()
			t.Cleanup(ResetImportAdapterRegistry)
			dir := t.TempDir()
			t.Chdir(dir)
			cache := filepath.Join(dir, "cache")
			t.Setenv("ATMOS_XDG_CACHE_HOME", cache)
			t.Setenv("XDG_CACHE_HOME", cache)
			provisioned := filepath.Join(cache, "atmos", "auth", "sso")
			require.NoError(t, os.MkdirAll(provisioned, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(provisioned, "provisioned-identities.yaml"), []byte(`auth:
  identities:
    developer:
      kind: aws/permission-set
      via: {provider: sso}
      principal: {name: provisioned, account: {id: "123456789012"}}
    other:
      kind: aws/permission-set
      via: {provider: sso}
      principal: {name: other, account: {id: "123456789012"}}
`), 0o600))
			files := map[string]string{
				"atmos.yaml":     "import: [manual.yaml]\n" + auth,
				"manual.yaml":    "auth:\n  identities:\n    developer:\n      principal:\n        name: manual\n",
				"providers.yaml": "sso:\n  kind: aws/iam-identity-center\n  region: us-east-2\n",
				"auth.yaml":      "providers:\n  sso:\n    kind: aws/iam-identity-center\n    region: us-east-2\nidentities:\n  developer:\n    default: true\n",
			}
			for name, content := range files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
			}
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, mergeConfig(v, dir, CliConfigFileName, true))
			require.Equal(t, "us-east-2", v.GetString("auth.providers.sso.region"))
			require.Equal(t, "aws/permission-set", v.GetString("auth.identities.developer.kind"))
			require.Equal(t, "manual", v.GetString("auth.identities.developer.principal.name"))
			require.True(t, v.GetBool("auth.identities.developer.default"))
			require.Equal(t, "other", v.GetString("auth.identities.other.principal.name"))
			t.Setenv("TEST_GIT_ROOT", dir)
			loaded, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
			require.NoError(t, err)
			require.Len(t, loaded.Auth.Identities, 2)
			developer := loaded.Auth.Identities["developer"]
			require.Equal(t, "aws/permission-set", developer.Kind)
			require.Equal(t, "manual", developer.Principal["name"])
			require.Equal(t, map[string]any{"id": "123456789012"}, developer.Principal["account"])
			require.True(t, developer.Default)
			other := loaded.Auth.Identities["other"]
			require.Equal(t, "aws/permission-set", other.Kind)
			require.Equal(t, "other", other.Principal["name"])
			require.NotNil(t, other.Via)
			require.Equal(t, "sso", other.Via.Provider)
		})
	}
}
