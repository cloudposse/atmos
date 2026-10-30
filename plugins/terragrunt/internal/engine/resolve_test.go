package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func nativeUnit(t *testing.T, extra string) model.Request {
	t.Helper()
	root := t.TempDir()
	module := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(module, 0o755))
	source, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte("terraform { source = "+string(source)+" }\n"+extra), 0o644))
	return model.Request{Path: path, BasePath: root, Command: "plan", Tofu: "must-not-execute", Environment: map[string]string{}}
}

func TestNativeLocalBackendAndLiteralGeneration(t *testing.T) {
	request := nativeUnit(t, `remote_state {
 backend = "local"
 config = { path = "relative/state.tfstate" }
}
generate "literal" {
 path = "literal.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = "# {{ keep }} $${literal}\n"
}
inputs = { large = 9007199254740993, nested = { nullable = null, enabled = false } }
`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(result.WorkingDir, "relative/state.tfstate"), result.Backend["path"])
	require.Equal(t, "# {{ keep }} ${literal}\n", result.Generated["literal.tf"])
	require.Equal(t, json.Number("9007199254740993"), result.Inputs["large"])
	require.NoFileExists(t, filepath.Join(filepath.Dir(request.Path), "literal.tf"))
}

func TestNativeRejectsUnsupportedSemantics(t *testing.T) {
	for _, test := range []struct{ name, extra, message string }{
		{"binary override", `terraform_binary = "another-tool"`, "terraform_binary"},
		{"IAM role", `iam_role = "unsupported"`, "IAM settings"},
		{"generation escape", `generate "x" {
 path = "../x.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = ""
}`, "unsupported generated path"},
		{"version constraint", `terragrunt_version_constraint = "> 99.0.0"`, "rejects version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(t.Context(), nativeUnit(t, test.extra))
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestBackendIdentityDoesNotExecuteVersionProbe(t *testing.T) {
	request := nativeUnit(t, `terraform_version_constraint = "> 99.0.0"
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
`)
	request.Phase = "backend"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "local", result.BackendType)
	request.Phase = ""
	_, err = Resolve(t.Context(), request)
	require.ErrorContains(t, err, "cannot inspect selected executable")
}

func TestDiscoveryDeniesCommands(t *testing.T) {
	request := nativeUnit(t, `locals { value = run_cmd("must-not-execute") }
inputs = { value = local.value }
`)
	request.Phase = "discover"
	_, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "read-only discovery cannot execute")
}

func TestVersionConstraintsUseSelectedExecutable(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, test := range []struct {
		name, actual, constraint string
		want                     string
	}{
		{"accepted", `{"terraform_version":"1.9.0"}`, ">= 1.8.0", ""},
		{"rejected", `{"terraform_version":"1.7.0"}`, ">= 1.8.0", "rejects version"},
		{"invalid version", `{"terraform_version":"invalid"}`, ">= 1.8.0", "malformed version"},
		{"invalid output", `not-json`, ">= 1.8.0", "invalid character"},
		{"invalid constraint", `{"terraform_version":"1.9.0"}`, "this is not a constraint", "malformed constraint"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := model.Request{Tofu: executable, Environment: map[string]string{"ATMOS_TERRAGRUNT_VERSION_CHILD": test.actual}}
			err := checkVersions(t.Context(), request, test.constraint, "")
			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.want)
			}
		})
	}
}

func TestResolveGuardsAndDownloadDirectory(t *testing.T) {
	request := nativeUnit(t, `download_dir = "custom-cache"
inputs = { value = "constant" }
generate "disabled" {
 path = "not-valid"
 disable = true
 if_exists = "overwrite"
 contents = "ignored"
}
`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "custom-cache")
	require.Empty(t, result.Generated)
	request.Environment["TG_DOWNLOAD_DIR"] = "environment-cache"
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "environment-cache")
	request.LogLevel = "not-a-level"
	_, err = Resolve(t.Context(), request)
	require.Error(t, err)
	request.LogLevel = ""
	request.Path = filepath.Join(request.BasePath, "missing.hcl")
	_, err = Resolve(t.Context(), request)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(request.Path, []byte("inputs = {}"), 0o644))
	_, err = Resolve(t.Context(), request)
	require.ErrorContains(t, err, "requires terraform.source")
	for _, source := range []string{"tfr://example.invalid/repo", "./absent"} {
		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(encoded)+" }"), 0o644))
		_, err = Resolve(t.Context(), request)
		require.Error(t, err)
	}
}
