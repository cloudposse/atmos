package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

const providerToolchainChildEnv = "ATMOS_TERRAGRUNT_PROVIDER_TOOLCHAIN_CHILD"

// TestMain turns this test binary into the fake Terraform/OpenTofu and HCL tool used below.
func TestMain(m *testing.M) {
	if os.Getenv(providerToolchainChildEnv) == "1" {
		if len(os.Args) > 1 && os.Args[1] == "version" {
			_, _ = os.Stdout.WriteString(`{"terraform_version":"1.9.0"}`)
			os.Exit(0)
		}
		executable, _ := os.Executable()
		directory, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"executable": executable, "directory": directory, "path": os.Getenv("PATH"), "profile": os.Getenv("AWS_PROFILE"), "token": os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"), "custom": os.Getenv("CUSTOM_VALUE")})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func installProviderExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(dir, name)
	executable, err := os.Executable()
	require.NoError(t, err)
	if err := os.Link(executable, target); err == nil {
		return target
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o755))
	return target
}

// The host sends the toolchain-resolved executable and PATH only in Invocation.Environment.
// This drives that exact wire shape through the real provider and evaluator.
func TestPrepareHonorsToolchainEnvironmentFromTheInvocation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	ambient := filepath.Join(root, "ambient")
	require.NoError(t, os.MkdirAll(ambient, 0o755))
	t.Setenv("PATH", ambient)
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	module := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(module, 0o755))
	unit := filepath.Join(root, "live", "app")
	require.NoError(t, os.MkdirAll(unit, 0o755))
	source, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(unit, "terragrunt.hcl"), []byte("terraform { source = "+string(source)+` }
terraform_version_constraint = ">= 1.8.0"
inputs = {
  tool   = jsondecode(run_cmd("fixture-tool"))
  custom = get_env("CUSTOM_VALUE", "unset")
}
`), 0o644))
	selected := installProviderExecutable(t, filepath.Join(root, "selected"), "tofu")
	toolDir := filepath.Join(root, "toolchain", "bin")
	tool := installProviderExecutable(t, toolDir, "fixture-tool")

	configuration, err := encode(map[string]any{})
	require.NoError(t, err)
	settings, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true}})
	require.NoError(t, err)
	prepare := func(profile string, environment map[string]string) (map[string]any, *pluginv1.PrepareComponentResponse, error) {
		environment["ATMOS_TERRAFORM_BINARY"] = selected
		environment[providerToolchainChildEnv] = "1"
		environment["AWS_PROFILE"] = profile
		request := &pluginv1.ComponentRequest{
			Provider: "terragrunt", RootDirectory: root, Source: "live/app/terragrunt.hcl", Configuration: configuration, ProviderConfiguration: settings,
			Operation:  &pluginv1.ComponentOperation{Mode: "native", Command: "plan"},
			Effects:    &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true},
			Invocation: &pluginv1.Invocation{Environment: environment},
		}
		response, err := New(nil).Prepare(t.Context(), request)
		if err != nil {
			return nil, nil, err
		}
		var inputs map[string]any
		require.NoError(t, decode(response.Inputs, &inputs))
		return inputs, response, nil
	}

	for _, profile := range []string{"development", "production"} {
		inputs, response, err := prepare(profile, map[string]string{"PATH": toolDir, "CUSTOM_VALUE": profile})
		require.NoError(t, err, profile)
		require.Equal(t, "opentofu", response.Engine, "the engine follows the selected executable")
		report := map[string]string{}
		for key, value := range inputs["tool"].(map[string]any) {
			report[key], _ = value.(string)
		}
		require.Equal(t, tool, report["executable"])
		require.Equal(t, toolDir, report["path"])
		require.Equal(t, profile, report["profile"])
		require.Equal(t, "", report["token"], "ambient credentials must not cross the plugin boundary")
		require.Equal(t, profile, inputs["custom"])
		require.Equal(t, unit, report["directory"], "commands run from the unit directory")
	}

	// Dropping PATH at the boundary must fail loudly instead of finding the ambient tool.
	t.Setenv("PATH", toolDir)
	_, _, err = prepare("development", map[string]string{"PATH": ambient})
	require.Error(t, err, "the evaluator error text is intentionally generic")
}
