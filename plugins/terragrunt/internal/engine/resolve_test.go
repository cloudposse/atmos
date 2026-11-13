package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
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
		{"generation escape", `generate "x" {
 path = "../x.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = ""
}`, "escapes the working directory"},
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
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, model.CodeEffectDenied, problem.Code)
	require.False(t, problem.Safe)
	require.NotEmpty(t, problem.Hints)
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
	require.ErrorContains(t, err, "terraform.source")
	for _, source := range []string{"ftp://example.invalid/repo", "./absent"} {
		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(encoded)+" }"), 0o644))
		_, err = Resolve(t.Context(), request)
		require.Error(t, err)
	}
}

// A unit that calls get_aws_account_id() sees the identity of the invocation, selected only through the
// invocation environment: the plugin process never holds the profile or the endpoint.
func TestNativeAWSFunctionsUseInvocationIdentity(t *testing.T) {
	var signer atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signer.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::210987654321:user/atmos</Arn><UserId>AIDAATMOS</UserId><Account>210987654321</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`)
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	credentials, configFile := filepath.Join(dir, "credentials"), filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(credentials, []byte("[atmos-identity]\naws_access_key_id = AKIAENGINE\naws_secret_access_key = secret\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile, nil, 0o600))
	// A known, valid process profile, so the test does not depend on the developer's own AWS_PROFILE.
	processCredentials, processConfig := filepath.Join(dir, "process-credentials"), filepath.Join(dir, "process-config")
	require.NoError(t, os.WriteFile(processCredentials, []byte("[process-profile]\naws_access_key_id = AKIAPROCESS\naws_secret_access_key = process\n"), 0o600))
	require.NoError(t, os.WriteFile(processConfig, nil, 0o600))
	t.Setenv("AWS_PROFILE", "process-profile")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", processCredentials)
	t.Setenv("AWS_CONFIG_FILE", processConfig)
	request := nativeUnit(t, "inputs = { account = get_aws_account_id(), arn = get_aws_caller_identity_arn(), user = get_aws_caller_identity_user_id() }\n")
	request.Environment = map[string]string{
		"AWS_PROFILE": "atmos-identity", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL_STS": server.URL, "AWS_REGION": "us-east-1", "AWS_EC2_METADATA_DISABLED": "true",
	}

	result, err := Resolve(t.Context(), request)

	require.NoError(t, err)
	require.Equal(t, "210987654321", result.Inputs["account"])
	require.Equal(t, "arn:aws:iam::210987654321:user/atmos", result.Inputs["arn"])
	require.Equal(t, "AIDAATMOS", result.Inputs["user"])
	require.Contains(t, signer.Load(), "Credential=AKIAENGINE/")
	require.Equal(t, "process-profile", os.Getenv("AWS_PROFILE"), "the plugin process keeps its own profile")
}

// Read-only discovery stays offline even when the invocation names credentials and an endpoint.
func TestDiscoveryNeverCallsAWS(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	request := nativeUnit(t, "inputs = { account = get_aws_account_id() }\n")
	request.Phase = "discover"
	request.Environment = map[string]string{"AWS_ACCESS_KEY_ID": "AKIAX", "AWS_SECRET_ACCESS_KEY": "x", "AWS_ENDPOINT_URL_STS": server.URL}

	// Discovery may succeed or fail on the function, depending on how much of the unit it evaluates.
	// Either way it must not reach the endpoint.
	_, _ = Resolve(t.Context(), request)

	require.Zero(t, calls.Load())
}

// Terragrunt discards the marks of the evaluated inputs, so the unit's own HCL decides which are sensitive.
func TestResolveReportsSensitiveInputs(t *testing.T) {
	request := nativeUnit(t, `locals {
  database = sensitive({ password = "pw", host = "db" })
  token    = sensitive("t0ken")
}
inputs = {
  direct   = sensitive("direct-secret")
  password = local.database.password
  token    = local.token
  host     = local.database.host
  plain    = "visible"
  nested   = { key = sensitive("nested-secret"), open = 1 }
  merged   = merge({ a = sensitive("m") }, { b = 2 })
  chosen   = true ? sensitive("c") : "d"
  wrapped  = "${sensitive("w")}"
  large    = 9007199254740993
}
`)

	result, err := Resolve(t.Context(), request)

	require.NoError(t, err)
	// A sensitive object makes every key of it sensitive, so host is sensitive as well.
	require.Equal(t, [][]string{{"chosen"}, {"direct"}, {"host"}, {"merged", "a"}, {"nested", "key"}, {"password"}, {"token"}, {"wrapped"}}, result.SensitiveInputs)
	require.Equal(t, json.Number("9007199254740993"), result.Inputs["large"])
	require.Equal(t, "visible", result.Inputs["plain"])
}

func TestSensitiveInputsFollowIncludesAndSkipWhatItCannotSee(t *testing.T) {
	fsys := afero.NewMemMapFs()
	write := func(path, content string) { require.NoError(t, afero.WriteFile(fsys, path, []byte(content), 0o644)) }
	write("/root.hcl", `inputs = { shared = sensitive("s"), open = "o" }`)
	write("/unit/terragrunt.hcl", `locals { cycle = local.cycle }
inputs = {
  own     = sensitive(1)
  looped  = local.cycle
  missing = local.absent
  indexed = local.cycle["k"]
  dynamic = { for k, v in { a = 1 } : k => sensitive(v) }
}`)
	write("/broken.hcl", `inputs = {`)
	write("/data.json", `{"inputs": {"json": "x"}}`)
	write("/whole.hcl", `inputs = sensitive({ everything = 1 })`)
	inputs := map[string]any{"shared": "s", "open": "o", "own": json.Number("1"), "looped": "x", "dynamic": map[string]any{"a": json.Number("1")}, "json": "x", "everything": json.Number("1")}

	files := []string{"/unit/terragrunt.hcl", "/root.hcl", "/broken.hcl", "/data.json", "/missing.hcl"}
	// A for expression that yields sensitive values is marked as a whole.
	require.Equal(t, [][]string{{"dynamic"}, {"looped"}, {"own"}, {"shared"}}, SensitiveInputs(fsys, files, inputs))

	require.Equal(t, [][]string{nil}, SensitiveInputs(fsys, []string{"/whole.hcl"}, inputs), "sensitive(...) around the whole inputs marks every input")
	require.Empty(t, SensitiveInputs(fsys, files, map[string]any{"open": "o"}), "a path that is not among the evaluated inputs is dropped")
	require.Empty(t, SensitiveInputs(fsys, nil, inputs))
}

func TestSensitiveInputsFollowExposedIncludeLocal(t *testing.T) {
	fsys := afero.NewMemMapFs()
	require.NoError(t, fsys.MkdirAll("/live/app", 0o755))
	require.NoError(t, afero.WriteFile(fsys, "/root.hcl", []byte(`locals {
  token = sensitive("secret")
  plain = "visible"
}`), 0o644))
	require.NoError(t, afero.WriteFile(fsys, "/live/app/terragrunt.hcl", []byte(`include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
}
inputs = {
  token = include.root.locals.token
  plain = include.root.locals.plain
}`), 0o644))
	paths := SensitiveInputs(fsys, []string{"/live/app/terragrunt.hcl", "/root.hcl"}, map[string]any{"token": "secret", "plain": "visible"})
	require.Equal(t, [][]string{{"token"}}, paths)
}

func TestSensitiveInputsFollowNestedLocalReferences(t *testing.T) {
	fsys := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fsys, "/unit.hcl", []byte(`locals {
  config = { token = sensitive("t"), open = "o", deeper = { key = sensitive("k") } }
  whole  = sensitive({ key = "v" })
  paren  = (sensitive({ inner = 1 }))
}
inputs = {
  token    = local.config.token
  open     = local.config.open
  key      = local.config.deeper.key
  wholekey = local.whole.key
  inner    = local.paren.inner
  indexed  = local.config["token"]
  missing  = local.config.nothing.deeper
  scalar   = local.config.open.length
  external = var.other
  call     = tostring(sensitive("c"))
  cond     = true ? local.config.token : "x"
}
`), 0o644))
	inputs := map[string]any{"token": "t", "open": "o", "key": "k", "wholekey": "v", "inner": 1, "indexed": "t", "missing": 1, "scalar": 1, "external": 1, "call": "c", "cond": "t"}

	// An index with a literal key follows the local like an attribute, and a function applied to a
	// sensitive value makes its whole result sensitive.
	require.Equal(t, [][]string{{"call"}, {"cond"}, {"indexed"}, {"inner"}, {"key"}, {"missing"}, {"token"}, {"wholekey"}}, SensitiveInputs(fsys, []string{"/unit.hcl"}, inputs))
}

func TestSensitiveInputsFailClosedWhenPathsCannotBeResolved(t *testing.T) {
	fsys := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fsys, "/unit.hcl", []byte(`locals {
  a = local.b
  b = local.c
  c = local.d
  d = local.e
  e = local.f
  f = local.g
  g = local.h
  h = local.i
  i = sensitive("secret")
  config = merge({ token = sensitive("secret") }, { open = "visible" })
}
inputs = {
  dynamic = { (local.key) = sensitive("secret") }
  chain = local.a
  wrapped = upper(local.a)
  composed = local.config.token
  plain = "visible"
}
`), 0o644))
	inputs := map[string]any{"dynamic": map[string]any{"key": "secret"}, "chain": "secret", "wrapped": "SECRET", "composed": "secret", "plain": "visible"}
	require.Equal(t, [][]string{{"chain"}, {"composed"}, {"dynamic"}, {"wrapped"}}, SensitiveInputs(fsys, []string{"/unit.hcl"}, inputs))
}

// A sensitive value that object keys cannot address marks the whole input, so it never reaches a varfile.
func TestSensitiveInputsMarkValuesTheyCannotAddress(t *testing.T) {
	fsys := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fsys, "/unit.hcl", []byte(`locals {
  token  = sensitive("t")
  tokens = [sensitive("a"), "b"]
}
inputs = {
  passwords = [sensitive("secret")]
  through   = [local.token]
  element   = local.tokens[0]
  encoded   = jsonencode({ key = sensitive("k") })
  generated = { for k, v in { a = 1 } : k => sensitive(v) }
  plain     = ["a", "b"]
  plaincall = upper("x")
}
`), 0o644))
	inputs := map[string]any{"passwords": []any{"secret"}, "through": []any{"t"}, "element": "a", "encoded": "{}", "generated": map[string]any{"a": 1}, "plain": []any{"a", "b"}, "plaincall": "X"}

	require.Equal(t, [][]string{{"element"}, {"encoded"}, {"generated"}, {"passwords"}, {"through"}}, SensitiveInputs(fsys, []string{"/unit.hcl"}, inputs))
}
