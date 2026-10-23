package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
