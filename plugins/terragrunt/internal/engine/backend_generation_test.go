package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
