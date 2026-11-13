package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudBackendIdentityWithoutNetwork(t *testing.T) {
	for _, test := range []struct {
		name, config, excluded string
	}{
		{"s3", `bucket = "existing-bucket", key = "env/state", region = "us-east-1", skip_bucket_versioning = true`, "skip_bucket_versioning"},
		{"gcs", `bucket = "existing-bucket", prefix = "env/state", skip_bucket_creation = true`, "skip_bucket_creation"},
		{"azurerm", `storage_account_name = "existing", container_name = "state", key = "env/state", resource_group_name = "existing", skip_storage_account_creation = true`, "skip_storage_account_creation"},
		{"http", `address = "http://127.0.0.1:1/state"`, "unused"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, `remote_state {
 backend = "`+test.name+`"
 config = { `+test.config+` }
}`)
			request.Phase = "backend"
			result, err := Resolve(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, test.name, result.BackendType)
			require.NotContains(t, result.Backend, test.excluded)
			require.NotEmpty(t, result.Backend)
			require.NoDirExists(t, filepath.Join(filepath.Dir(request.Path), ".terragrunt-cache"))
		})
	}
}

func TestLocalBackendWithoutConfigUsesDefaults(t *testing.T) {
	request := nativeUnit(t, `remote_state {
 backend = "local"
 config = null
}`)
	request.Phase = "backend"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(result.WorkingDir, "terraform.tfstate"), result.Backend["path"])
	require.Equal(t, filepath.Join(result.WorkingDir, "terraform.tfstate.d"), result.Backend["workspace_dir"])
}

func TestRemoteBackendGenerationRetainsPathAndPolicy(t *testing.T) {
	request := nativeUnit(t, `remote_state {
 backend = "s3"
 generate = { path = "custom/backend.tf", if_exists = "overwrite_terragrunt" }
 config = {
  bucket = "existing-bucket"
  key = "env/state"
  region = "us-east-1"
  skip_bucket_versioning = true
  assume_role = { role_arn = "arn:aws:iam::123456789012:role/state" }
 }
}`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.True(t, result.BackendGenerated)
	path := filepath.Join("custom", "backend.tf")
	require.Equal(t, "overwrite_terragrunt", result.GeneratedPolicies[path])
	require.True(t, result.SensitiveGenerated[path], "a generated backend can hold credentials")
	require.Contains(t, result.Generated[path], `backend "s3"`)
	require.Contains(t, result.Generated[path], `assume_role = {`)
	require.NotContains(t, result.Generated[path], "skip_bucket_versioning")
	require.IsType(t, map[string]any{}, result.Backend["assume_role"])
	require.NoDirExists(t, filepath.Join(filepath.Dir(request.Path), "custom"))
}

func TestUnsupportedBackendSemanticsAndPaths(t *testing.T) {
	for _, test := range []struct{ name, hcl, want string }{
		{"disable_init", `remote_state {
 backend = "s3"
 disable_init = true
 config = { bucket = "existing" }
}`, "disable_init"},
		{"encryption", `remote_state {
 backend = "s3"
 encryption = { key_provider = "pbkdf2", passphrase = "0123456789abcdef" }
 config = { bucket = "existing" }
}`, "disable_init"},
		{"backend path escapes", `remote_state {
 backend = "local"
 generate = { path = "../escape.tf", if_exists = "overwrite" }
 config = { path = "state" }
}`, "escapes the working directory"},
		{"backend path is the directory", `remote_state {
 backend = "local"
 generate = { path = ".", if_exists = "overwrite" }
 config = { path = "state" }
}`, "escapes the working directory"},
		{"backend path duplicates generate block", `remote_state {
 backend = "local"
 generate = { path = "same.tf", if_exists = "overwrite" }
 config = { path = "state" }
}
generate "duplicate" {
 path = "same.tf"
 if_exists = "overwrite"
 contents = ""
}`, "write the same file"},
		{"backend path duplicates a normalized generate path", `remote_state {
 backend = "local"
 generate = { path = "./same.tf", if_exists = "overwrite" }
 config = { path = "state" }
}
generate "duplicate" {
 path = "same.tf"
 if_exists = "overwrite"
 contents = ""
}`, "write the same file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Resolve(t.Context(), nativeUnit(t, test.hcl))
			require.ErrorContains(t, err, test.want)
			require.Equal(t, test.name == "disable_init" || test.name == "encryption", errors.Is(err, ErrUnsupportedRemoteState))
			require.Nil(t, result)
		})
	}
}

func TestGeneratedBackendIdentityRejectedAcrossPhases(t *testing.T) {
	for _, phase := range []string{"", "backend"} {
		for _, backend := range []string{
			"terraform {\n backend \"local\" { path = \"other.tfstate\" }\n}",
			"terraform {\n backend \"s3\" {\n bucket = \"other-bucket\"\n key = \"state\"\n region = \"us-east-1\"\n }\n}",
			"terraform {\n cloud { organization = \"example\" }\n}",
		} {
			for _, remote := range []string{"", "remote_state {\n backend = \"local\"\n config = { path = \"known.tfstate\" }\n}\n"} {
				t.Run(phase+backend+remote, func(t *testing.T) {
					request := nativeUnit(t, remote+fmt.Sprintf("generate \"backend\" {\n path = \"custom.tf\"\n if_exists = \"skip\"\n contents = %q\n}", backend))
					request.Phase = phase
					owned := filepath.Join(filepath.Dir(request.Path), "custom.tf")
					require.NoError(t, os.WriteFile(owned, []byte("user-owned"), 0o600))
					result, err := Resolve(t.Context(), request)
					require.ErrorIs(t, err, ErrUnsupportedExecution)
					require.ErrorContains(t, err, "Cannot verify the backend identity")
					require.Nil(t, result)
					bytes, err := os.ReadFile(owned)
					require.NoError(t, err)
					require.Equal(t, "user-owned", string(bytes))
					require.NoDirExists(t, filepath.Join(filepath.Dir(request.Path), ".terragrunt-cache"))
				})
			}
		}
	}
}

func TestBackendGenerationPartialParseFailsClosed(t *testing.T) {
	for name, extra := range map[string]string{
		"json":    "path = \"backend.tf.json\"\n contents = \"{\\\"terraform\\\":{\\\"backend\\\":{\\\"s3\\\":{}}}}\"",
		"dynamic": "path = \"backend.tf\"\n contents = run_cmd(\"must-not-execute\")",
		"path":    "path = run_cmd(\"must-not-execute\")\n contents = \"# harmless\"",
		"disable": "path = \"backend.tf\"\n disable = run_cmd(\"must-not-execute\")\n contents = \"# harmless\"",
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, "generate \"x\" {\n if_exists = \"overwrite\"\n"+extra+"\n}\ninputs = { value = run_cmd(\"must-not-evaluate-inputs\") }")
			request.Phase = "backend"
			result, err := Resolve(t.Context(), request)
			require.ErrorContains(t, err, "Cannot verify the backend identity")
			require.NotContains(t, err.Error(), "must-not")
			require.Nil(t, result)
		})
	}
}

func TestBackendGenerationPartialParseInspectsIncludes(t *testing.T) {
	request := nativeUnit(t, "include \"parent\" { path = \"parent.hcl\" }")
	request.Phase = "backend"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), "parent.hcl"), []byte("generate \"backend\" {\n path = \"backend.tf\"\n if_exists = \"overwrite\"\n contents = \"terraform {\\n backend \\\"local\\\" {}\\n}\"\n}"), 0o600))
	result, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "Cannot verify the backend identity")
	require.Nil(t, result)
}

func TestBackendGenerationPartialParsePreservesResolvedIdentity(t *testing.T) {
	for _, backend := range []string{"local", "s3"} {
		request := nativeUnit(t, fmt.Sprintf(`remote_state {
 backend = %q
 generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
 config = { bucket = "known", key = "state", region = "us-east-1" }
}
generate "provider" {
 path = "provider.tf"
 if_exists = "overwrite"
 contents = "provider \"aws\" {}"
}
generate "disabled" {
 path = "old.tf"
 if_exists = "overwrite"
 disable = true
 contents = run_cmd("must-not-evaluate-disabled")
}
inputs = { value = run_cmd("must-not-evaluate-inputs") }
`, backend))
		request.Phase = "backend"
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, backend, result.BackendType)
		require.True(t, result.BackendGenerated)
		require.Contains(t, result.Generated["backend.tf"], "backend \""+backend+"\"")
	}
}

func TestBackendGenerationPartialParseRejectsAttributeForm(t *testing.T) {
	request := nativeUnit(t, `generate = {
 backend = {
  path = "backend.tf"
  if_exists = "overwrite"
  contents = "terraform {\n backend \"local\" {}\n}"
 }
}`)
	request.Phase = "backend"
	result, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "Cannot verify the backend identity")
	require.Nil(t, result)
}

func TestBackendGenerationPartialParseInspectsAutoIncludes(t *testing.T) {
	request := nativeUnit(t, "")
	request.Phase = "backend"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), "terragrunt.autoinclude.hcl"), []byte(`generate "backend" {
 path = "backend.tf"
 if_exists = "overwrite"
 contents = "terraform {\n backend \"local\" {}\n}"
}`), 0o600))
	result, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "Cannot verify backend identity from an automatic include")
	require.Nil(t, result)
}
