package scoped

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

var (
	_ = schema.AtmosConfiguration{StackConfigFilesAbsolutePaths: nil, StacksBaseAbsolutePath: ""}
	_ = schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}
	_ = schema.Stacks{NameTemplate: ""}
)

func TestResolveProviders_OnlyRequiredFields(t *testing.T) {
	t.Setenv("ATMOS_TEST_STACK_PROVIDER_PATH", "secrets/dev.enc.yaml")
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
	section := map[string]any{
		"vars": map[string]any{"directory": "secrets", "unused": "{{ atmos.Component \"missing\" \"dev\" }}"},
		"secrets": map[string]any{
			"providers": map[string]any{"local": map[string]any{"kind": "sops/age", "spec": map[string]any{
				"file": (yamlsource.Source{Tag: "!env", Argument: "ATMOS_TEST_STACK_PROVIDER_PATH"}).Encode(), "age_key_file": "{{ .vars.directory }}/keys.txt",
			}}},
			"vars": map[string]any{"TOKEN": (yamlsource.Source{Tag: "!secret", Argument: "UNINITIALIZED"}).Encode()},
		},
	}
	resolved, err := resolveProviders(config, "dev", section, []string{"secret", "terraform.output", "terraform.state", "store"})
	require.NoError(t, err)
	provider := secrets.ExtractProviders(resolved)["local"].(map[string]any)
	require.Equal(t, map[string]any{"file": "secrets/dev.enc.yaml", "age_key_file": "secrets/keys.txt"}, provider["spec"])
	require.Empty(t, secrets.ExtractDeclarations(resolved))
	require.NotContains(t, resolved["vars"], "unused")
	require.Equal(t, (yamlsource.Source{Tag: "!env", Argument: "ATMOS_TEST_STACK_PROVIDER_PATH"}).Encode(), secrets.ExtractProviders(section)["local"].(map[string]any)["spec"].(map[string]any)["file"])
}

const providerExitHelperEnv = "_ATMOS_SCOPED_PROVIDER_EXIT_HELPER"

// TestMain lets the test binary act as a portable failing subprocess.
func TestMain(m *testing.M) {
	if os.Getenv(providerExitHelperEnv) == "1" {
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func TestResolveProviders_PropagatesEvaluationErrors(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	command, err := syntax.Quote(executable, syntax.LangBash)
	require.NoError(t, err)
	t.Setenv(providerExitHelperEnv, "1")

	for _, tc := range []struct {
		name             string
		value            string
		expectedExitCode int
	}{
		{name: "invalid template", value: "{{ invalidFunction }}"},
		{name: "exec exit code", value: (yamlsource.Source{Tag: "!exec", Argument: command}).Encode(), expectedExitCode: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
			section := map[string]any{"secrets": map[string]any{"providers": map[string]any{"local": map[string]any{"kind": tc.value}}}}
			_, err := resolveProviders(config, "dev", section, nil)
			require.Error(t, err)
			if tc.expectedExitCode != 0 {
				var exitErr errUtils.ExitCodeError
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, tc.expectedExitCode, exitErr.Code)
			}
		})
	}
}

func TestStackProviderSections_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte("secrets: [invalid"), 0o644))
	config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: dir, StackConfigFilesAbsolutePaths: []string{path}}
	_, err := StackProviderSections(config, "dev", nil)
	require.Error(t, err)
}

func TestStackProviderSections_InvalidConfiguration(t *testing.T) {
	for _, field := range []string{"name", "provider", "settings"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dev.yaml")
			manifest := "name: dev\nsecrets:\n  providers:\n    local:\n      kind: sops/age\n"
			config := &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: dir, StackConfigFilesAbsolutePaths: []string{path},
				Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}},
			}
			switch field {
			case "name":
				manifest = "secrets:\n  providers:\n    local:\n      kind: sops/age\n"
				config.Stacks.NameTemplate = "{{ invalidFunction }}"
			case "provider":
				manifest += "      spec:\n        file: '{{ invalidFunction }}'\n"
			case "settings":
				manifest += "settings: 123\n"
			}
			require.NoError(t, os.WriteFile(path, []byte(manifest), 0o644))
			_, err := StackProviderSections(config, "dev", nil)
			require.Error(t, err)
		})
	}
}
