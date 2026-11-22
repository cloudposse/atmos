package engine

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2/hclparse"
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
		for backendIndex, backend := range []string{
			"terraform {\n backend \"local\" { path = \"other.tfstate\" }\n}",
			"terraform {\n backend \"s3\" {\n bucket = \"other-bucket\"\n key = \"state\"\n region = \"us-east-1\"\n }\n}",
			"terraform {\n cloud { organization = \"example\" }\n}",
		} {
			for remoteIndex, remote := range []string{"", "remote_state {\n backend = \"local\"\n config = { path = \"known.tfstate\" }\n}\n"} {
				// The subtest name becomes part of its temp directory, so it must stay free of the
				// HCL text and its glob metacharacters.
				t.Run(fmt.Sprintf("phase-%s-backend-%d-remote-%d", cmp.Or(phase, "default"), backendIndex, remoteIndex), func(t *testing.T) {
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
	require.ErrorContains(t, err, "Cannot verify the backend identity")
	require.ErrorIs(t, err, ErrUnsupportedExecution)
	require.Nil(t, result)
}

// An automatic include does not block a state read by existing: only what it generates matters.
func TestBackendGenerationAcceptsAutoIncludeWithoutBackendGeneration(t *testing.T) {
	request := nativeUnit(t, "")
	request.Phase = "backend"
	auto := filepath.Join(filepath.Dir(request.Path), "terragrunt.autoinclude.hcl")
	require.NoError(t, os.WriteFile(auto, []byte("inputs = {\n  extra = \"value\"\n}\n"), 0o600))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NoError(t, os.WriteFile(auto, []byte("generate \"extra\" {\n path = \"extra.txt\"\n if_exists = \"overwrite\"\n contents = \"data\"\n}\n"), 0o600))
	_, err = Resolve(t.Context(), request)
	require.NoError(t, err, "a literal file that cannot declare a backend is verified")

	require.NoError(t, os.WriteFile(auto, []byte("generate \"extra\" {\n path = get_env(\"TARGET\")\n if_exists = \"overwrite\"\n contents = \"data\"\n}\n"), 0o600))
	request.Environment["TARGET"] = "extra.txt"
	_, err = Resolve(t.Context(), request)
	require.ErrorIs(t, err, ErrUnsupportedExecution, "a target that needs evaluation cannot be verified")
}

// A literal generate map gets the same checks as named generate blocks instead of being rejected.
func TestBackendGenerationChecksLiteralGenerateMaps(t *testing.T) {
	tests := []struct {
		name, attribute string
		rejected        bool
	}{
		{name: "non-terraform file", attribute: `generate = { notes = { path = "notes.txt", if_exists = "overwrite", contents = "backend" } }`},
		{name: "terraform file without a backend", attribute: `generate = { vars = { path = "vars.tf", if_exists = "overwrite", contents = "variable \"a\" {}" } }`},
		{name: "disabled entry", attribute: `generate = { backend = { path = "backend.tf", disable = true, contents = "terraform { backend \"local\" {} }" } }`},
		{name: "declares a backend", attribute: `generate = { backend = { path = "backend.tf", if_exists = "overwrite", contents = "terraform {\n backend \"local\" {}\n}" } }`, rejected: true},
		{name: "dynamic path", attribute: `generate = { backend = { path = get_env("TARGET"), if_exists = "overwrite", contents = "x" } }`, rejected: true},
		{name: "dynamic contents", attribute: `generate = { backend = { path = "backend.tf", if_exists = "overwrite", contents = get_env("BODY") } }`, rejected: true},
		{name: "dynamic map", attribute: `generate = get_env("MAP")`, rejected: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, test.attribute)
			request.Phase = "backend"
			request.Environment["TARGET"], request.Environment["BODY"], request.Environment["MAP"] = "backend.tf", "x", "{}"
			_, err := Resolve(t.Context(), request)
			if test.rejected {
				require.ErrorIs(t, err, ErrUnsupportedExecution)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBackendGenerationIgnoresDataWatches(t *testing.T) {
	for _, expression := range []string{`mark_as_read("optional.txt")`, `read_tfvars_file("values.json")`} {
		t.Run(expression, func(t *testing.T) {
			request := nativeUnit(t, "locals { data = "+expression+" }\n")
			request.Phase = "backend"
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), "values.json"), []byte(`{"generate":{"path":"backend.tf","contents":"invalid"}}`), 0o600))
			_, err := Resolve(t.Context(), request)
			require.NoError(t, err)
		})
	}
}

func TestBackendGenerationNormalizesTargetPath(t *testing.T) {
	for _, target := range []string{"backend.tf/.", "./backend.tf", "backend.tf.json/."} {
		t.Run(target, func(t *testing.T) {
			content := "terraform {\n backend \"local\" { path = \"other.tfstate\" }\n}"
			if target == "backend.tf.json/." {
				content = `{"terraform":{"backend":{"local":{"path":"other.tfstate"}}}}`
			}
			request := nativeUnit(t, fmt.Sprintf("generate \"backend\" {\n path = %q\n if_exists = \"overwrite\"\n contents = %q\n}", target, content))
			request.Phase = "backend"
			_, err := Resolve(t.Context(), request)
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}

func TestOpenTofuGeneratedBackendRejected(t *testing.T) {
	for _, phase := range []string{"", "backend"} {
		for _, target := range []string{"backend.tofu", "backend.tofu.json"} {
			t.Run(phase+target, func(t *testing.T) {
				content := "terraform {\n backend \"local\" { path = \"other.tfstate\" }\n}"
				if filepath.Ext(target) == ".json" {
					content = `{"terraform":{"backend":{"local":{"path":"other.tfstate"}}}}`
				}
				request := nativeUnit(t, fmt.Sprintf("generate \"backend\" {\n path = %q\n if_exists = \"overwrite\"\n disable_signature = true\n contents = %q\n}", target, content))
				request.Phase = phase
				_, err := Resolve(t.Context(), request)
				require.ErrorIs(t, err, ErrUnsupportedExecution)
			})
		}
	}
}

// A JSON generate path is read without evaluation, so a template such as ${"backend.tf"} reaches the
// check literally. It could name a backend file once evaluated, so it fails closed.
func TestBackendGenerationRejectsInterpolatedJSONPaths(t *testing.T) {
	for name, test := range map[string]struct {
		path string
		safe bool
	}{
		"interpolated": {path: `${\"backend.tf\"}`},
		"directive":    {path: `%{if true}backend.tf%{endif}`},
		"literal":      {path: "extra.txt", safe: true},
	} {
		t.Run(name, func(t *testing.T) {
			content := `{"generate": {"extra": {"path": "` + test.path + `", "if_exists": "overwrite", "contents": "terraform { backend \"s3\" {} }"}}}`
			file, diagnostics := hclparse.NewParser().ParseJSON([]byte(content), "terragrunt.hcl.json")
			require.False(t, diagnostics.HasErrors(), diagnostics.Error())
			err := checkBackendGenerationFile(file)
			if test.safe {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}

// A generate block whose literal path and contents declare a backend fails discovery, so bulk
// preflight stops before a healthy prerequisite runs. A declaration that needs evaluation, or one
// that writes no backend, still discovers.
func TestDiscoveryRejectsLiteralGeneratedBackends(t *testing.T) {
	for name, test := range map[string]struct {
		generate string
		rejected bool
	}{
		"literal backend":  {generate: "generate \"backend\" {\n path = \"backend.tf\"\n if_exists = \"overwrite\"\n contents = \"terraform {\\n backend \\\"local\\\" {}\\n}\\n\"\n}\n", rejected: true},
		"dynamic path":     {generate: "generate \"backend\" {\n path = get_env(\"TARGET\", \"backend.tf\")\n if_exists = \"overwrite\"\n contents = \"terraform {\\n backend \\\"local\\\" {}\\n}\\n\"\n}\n"},
		"disabled backend": {generate: "generate \"backend\" {\n path = \"backend.tf\"\n if_exists = \"overwrite\"\n disable = true\n contents = \"terraform {\\n backend \\\"local\\\" {}\\n}\\n\"\n}\n"},
		"provider only":    {generate: "generate \"provider\" {\n path = \"provider.tf\"\n if_exists = \"overwrite\"\n contents = \"provider \\\"null\\\" {}\\n\"\n}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, test.generate)
			request.Phase = "discover"
			_, err := Resolve(t.Context(), request)
			if !test.rejected {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrUnsupportedExecution)
			var problem *model.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "generate", problem.Key)
		})
	}
}

// Terragrunt records a no_merge include but never merges its generate settings, so its backend scan
// is skipped. Merged strategies, and an unset one, keep scanning.
func TestBackendConfigurationPathsSkipNoMergeIncludes(t *testing.T) {
	noMerge, deep, invalid := "no_merge", "deep", "bogus"
	cfg := &config.TerragruntConfig{ProcessedIncludes: config.IncludeConfigsMap{
		"a_unset":   {Path: "unset.hcl"},
		"b_nomerge": {Path: "nomerge.hcl", MergeStrategy: &noMerge},
		"c_deep":    {Path: "deep.hcl", MergeStrategy: &deep},
		"d_invalid": {Path: "invalid.hcl", MergeStrategy: &invalid},
	}}
	root := filepath.Join(string(filepath.Separator), "unit")
	unit := filepath.Join(root, "terragrunt.hcl")
	require.Equal(t, []string{
		unit,
		filepath.Join(root, "unset.hcl"),
		filepath.Join(root, "deep.hcl"),
		filepath.Join(root, "invalid.hcl"),
	}, backendConfigurationPaths(cfg, unit))
}

// Each entry of a literal generate map is judged on its own: a dynamic entry is skipped during
// discovery, and a literal backend entry beside it is still found.
func TestDiscoveryFindsLiteralBackendBesideDynamicGenerateEntry(t *testing.T) {
	backend := `backend = { path = "backend.tf", if_exists = "overwrite", contents = "terraform {\n backend \"local\" {}\n}\n" }`
	notes := `notes = { path = "notes.txt", if_exists = "overwrite", contents = get_env("NOTE", "x") }`
	provider := `provider = { path = "provider.tf", if_exists = "overwrite", contents = "provider \"null\" {}\n" }`
	for name, test := range map[string]struct {
		entries  string
		rejected bool
	}{
		"literal backend after dynamic":  {entries: notes + "\n" + backend, rejected: true},
		"literal backend before dynamic": {entries: backend + "\n" + notes, rejected: true},
		"dynamic and provider only":      {entries: notes + "\n" + provider},
		"dynamic only":                   {entries: notes},
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, "generate = {\n"+test.entries+"\n}\n")
			request.Phase = "discover"
			_, err := Resolve(t.Context(), request)
			if !test.rejected {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}

// JSON syntax is read without an evaluation context, so contents may reach the check as an
// unevaluated template. It cannot be proven free of a backend, so it fails closed; escaped
// template text and plain provider contents stay static.
func TestBackendGenerationRejectsUnevaluatedJSONContents(t *testing.T) {
	for name, test := range map[string]struct {
		contents string
		safe     bool
	}{
		"interpolation":    {contents: `${\"terraform { backend {} }\"}`},
		"directive":        {contents: `%{if true}provider {}%{endif}`},
		"provider only":    {contents: `provider \"null\" {}`, safe: true},
		"escaped template": {contents: `# $${literal} and %%{literal}`, safe: true},
		"literal backend":  {contents: `terraform {\n backend \"s3\" {}\n}`},
	} {
		t.Run(name, func(t *testing.T) {
			content := `{"generate": {"extra": {"path": "extra.tf", "if_exists": "overwrite", "contents": "` + test.contents + `"}}}`
			file, diagnostics := hclparse.NewParser().ParseJSON([]byte(content), "terragrunt.hcl.json")
			require.False(t, diagnostics.HasErrors(), diagnostics.Error())
			err := checkBackendGenerationFile(file)
			if test.safe {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}
