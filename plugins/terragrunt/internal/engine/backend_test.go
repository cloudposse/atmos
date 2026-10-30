package engine

import (
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
		{"disable_init", `remote_state {
 backend = "s3"
 disable_init = true
 config = { bucket = "existing" }
}`, "disable_init or encryption is not implemented"},
		{"encryption", `remote_state {
 backend = "s3"
 encryption = { key_provider = "pbkdf2", passphrase = "0123456789abcdef" }
 config = { bucket = "existing" }
}`, "disable_init or encryption is not implemented"},
		{"backend path escapes", `remote_state {
 backend = "local"
 generate = { path = "../escape.tf", if_exists = "overwrite" }
 config = { path = "state" }
}`, "unsupported backend generated path: ../escape.tf"},
		{"backend path is the directory", `remote_state {
 backend = "local"
 generate = { path = ".", if_exists = "overwrite" }
 config = { path = "state" }
}`, "unsupported backend generated path: ."},
		{"backend path duplicates generate block", `remote_state {
 backend = "local"
 generate = { path = "same.tf", if_exists = "overwrite" }
 config = { path = "state" }
}
generate "duplicate" {
 path = "same.tf"
 if_exists = "overwrite"
 contents = ""
}`, "duplicate backend generated path: same.tf"},
		{"backend path duplicates a normalized generate path", `remote_state {
 backend = "local"
 generate = { path = "./same.tf", if_exists = "overwrite" }
 config = { path = "state" }
}
generate "duplicate" {
 path = "same.tf"
 if_exists = "overwrite"
 contents = ""
}`, "duplicate backend generated path: ./same.tf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Resolve(t.Context(), nativeUnit(t, test.hcl))
			require.ErrorContains(t, err, test.want)
			require.Nil(t, result)
		})
	}
}
