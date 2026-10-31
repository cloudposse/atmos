package engine

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
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
	require.Contains(t, result.Generated[path], `backend "s3"`)
	require.Contains(t, result.Generated[path], `assume_role = {`)
	require.NotContains(t, result.Generated[path], "skip_bucket_versioning")
	require.IsType(t, map[string]any{}, result.Backend["assume_role"])
	require.NoDirExists(t, filepath.Join(filepath.Dir(request.Path), "custom"))
}

func TestUnsupportedBackendSemanticsAndPaths(t *testing.T) {
	for _, test := range []struct{ name, hcl, want string }{
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
			require.Nil(t, result)
		})
	}
}

const pbkdf2Passphrase = "correct-horse-battery-staple"

func encryptedBackend(generate string) string {
	return `remote_state {
 backend = "local"
 ` + generate + `
 encryption = { key_provider = "pbkdf2", passphrase = "` + pbkdf2Passphrase + `" }
 config = { path = "state.tfstate" }
}`
}

func TestRemoteStateEncryptionIsWrittenToGeneratedBackend(t *testing.T) {
	request := nativeUnit(t, encryptedBackend(`generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }`))
	request.Tofu = "/opt/bin/tofu"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.True(t, result.BackendGenerated)
	generated := result.Generated["backend.tf"]
	require.Contains(t, generated, `backend "local"`)
	require.Contains(t, generated, "encryption {")
	require.Contains(t, generated, `key_provider "pbkdf2" "default"`)
	require.Contains(t, generated, "passphrase = \""+pbkdf2Passphrase+"\"")
	require.Empty(t, result.Warnings)
	require.NoFileExists(t, filepath.Join(filepath.Dir(request.Path), "backend.tf"))
}

func TestRemoteStateEncryptionRejectsTerraform(t *testing.T) {
	request := nativeUnit(t, encryptedBackend(`generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }`))
	request.Tofu = "/opt/bin/terraform"
	_, err := Resolve(t.Context(), request)
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "remote_state.encryption", problem.Key)
	require.Contains(t, problem.Summary, "OpenTofu")
	require.NotEmpty(t, problem.Hints)
	require.NotContains(t, problem.Error(), pbkdf2Passphrase)

	// State identity does not write the block, so it is not blocked.
	request.Phase = "backend"
	_, err = Resolve(t.Context(), request)
	require.NoError(t, err)
}

func TestRemoteStateEncryptionWithoutGenerateWarns(t *testing.T) {
	request := nativeUnit(t, encryptedBackend(""))
	request.Tofu = "/opt/bin/terraform"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.False(t, result.BackendGenerated)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, warningEncryptionNotGenerated, result.Warnings[0].Code)
	require.NotContains(t, result.Warnings[0].Message, pbkdf2Passphrase)
}

func TestRemoteStateDisableInitDoesNotChangeTheBackend(t *testing.T) {
	const backend = `remote_state {
 backend = "s3"
 %s
 generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
 config = { bucket = "existing-bucket", key = "env/state", region = "us-east-1", skip_bucket_versioning = true }
}`
	plain, err := Resolve(t.Context(), nativeUnit(t, fmt.Sprintf(backend, "")))
	require.NoError(t, err)
	disabled, err := Resolve(t.Context(), nativeUnit(t, fmt.Sprintf(backend, "disable_init = true")))
	require.NoError(t, err)
	require.Equal(t, plain.BackendType, disabled.BackendType)
	require.Equal(t, plain.Backend, disabled.Backend)
	require.Contains(t, disabled.Generated["backend.tf"], `backend "s3"`)
	require.Equal(t, plain.Generated["backend.tf"], disabled.Generated["backend.tf"])
	require.True(t, disabled.BackendGenerated)
}
