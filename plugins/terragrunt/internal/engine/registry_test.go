package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// registryServer serves the Terraform module registry protocol for acme/vpc/aws over TLS and counts requests.
func registryServer(t *testing.T) (host string, requests *atomic.Int32) {
	t.Helper()
	requests = &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"modules.v1":"/v1/modules/"}`)
	})
	mux.HandleFunc("/v1/modules/acme/vpc/aws/versions", func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"modules":[{"versions":[{"version":"1.0.0"},{"version":"1.4.0"},{"version":"2.0.0"}]}]}`)
	})
	mux.HandleFunc("/v1/modules/acme/vpc/aws/", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("X-Terraform-Get", "github.com/acme/terraform-aws-vpc?ref=v"+filepath.Base(filepath.Dir(r.URL.Path)))
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	previous := registryClient
	registryClient = server.Client()
	t.Cleanup(func() { registryClient = previous })
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	return address.Host, requests
}

func registryUnit(t *testing.T, terraformBody string) model.Request {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte("terraform {\n"+terraformBody+"\n}\n"), 0o644))
	return model.Request{Path: path, BasePath: root, Command: "plan", Tofu: "/bin/tofu", Environment: map[string]string{"HOME": root}}
}

func TestRegistryDiscoveryRetriesTransientFailure(t *testing.T) {
	var discoveryCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		if discoveryCalls.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"modules.v1":"/v1/modules/"}`)
	})
	mux.HandleFunc("/v1/modules/acme/vpc/aws/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Terraform-Get", "https://example.test/module.zip")
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	previous := registryClient
	registryClient = server.Client()
	defer func() { registryClient = previous }()
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	request := registryUnit(t, `source = "tfr://`+address.Host+`/acme/vpc/aws?version=1.0.0"`)
	file := `terraform {
  source = "tfr://` + address.Host + `/acme/vpc/aws?version=1.0.0"
}
errors {
  retry "registry" {
    retryable_errors = ["status code 503"]
    max_attempts = 2
    sleep_interval_sec = 0
  }
}`
	require.NoError(t, os.WriteFile(request.Path, []byte(file), 0o644))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "https://example.test/module.zip", result.Source)
	require.EqualValues(t, 2, discoveryCalls.Load())
}

// failingRegistryUnit writes a unit whose registry always answers 503 and counts the discovery requests.
func failingRegistryUnit(t *testing.T, policies string) (request model.Request, discoveryCalls *atomic.Int32) {
	t.Helper()
	discoveryCalls = &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		discoveryCalls.Add(1)
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	previous := registryClient
	registryClient = server.Client()
	t.Cleanup(func() { registryClient = previous })
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	source := "tfr://" + address.Host + "/acme/vpc/aws?version=1.0.0"
	request = registryUnit(t, `source = "`+source+`"`)
	file := "terraform {\n  source = \"" + source + "\"\n}\nerrors {\n" + policies + "\n}\n"
	require.NoError(t, os.WriteFile(request.Path, []byte(file), 0o644))
	return request, discoveryCalls
}

const registryRetryPolicy = `  retry "registry" {
    retryable_errors = ["status code 503"]
    max_attempts = 3
    sleep_interval_sec = 0
  }`

// A failure that matches an ignore policy and a retry policy is ignored on the first attempt: ignore
// policies are read before retry policies, and the unit keeps the source it was written with.
func TestRegistryResolutionFailureMatchingIgnoreAndRetryIsIgnoredWithoutRetrying(t *testing.T) {
	request, discoveryCalls := failingRegistryUnit(t, registryRetryPolicy+`
  ignore "registry_down" {
    ignorable_errors = ["status code 503"]
    message = "registry is down"
  }`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.EqualValues(t, 1, discoveryCalls.Load(), "an ignored failure must not be retried")
	require.NotNil(t, result.RunIgnored)
	assert.Equal(t, model.IgnoredRun{Policy: "registry_down", Message: "registry is down"}, *result.RunIgnored)
	assert.Contains(t, result.Source, "tfr://", "no source was resolved, so the unit keeps the source it was written with")
	assert.Nil(t, result.RegistryAuth)
	assert.NotEmpty(t, result.WorkingDir)
}

// A failure no ignore policy matches is retried exactly as before and then fails.
func TestRegistryResolutionFailureNotIgnoredIsStillRetried(t *testing.T) {
	request, discoveryCalls := failingRegistryUnit(t, registryRetryPolicy+`
  ignore "unrelated" {
    ignorable_errors = ["certificate has expired"]
  }`)
	_, err := Resolve(t.Context(), request)
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	assert.Equal(t, model.CodeInvalidSource, problem.Code)
	assert.EqualValues(t, 3, discoveryCalls.Load(), "a failure that no ignore policy matches is retried up to max_attempts")
}

// Retry patterns match the same cleaned text as ignore patterns, so one pattern behaves alike in both
// lists, like a source fetch: a message split across lines, or with characters outside the readable
// set, still matches a pattern written for its readable form.
func TestRetryRegistryResolutionMatchesRetryPatternsAgainstCleanedText(t *testing.T) {
	policies := &model.Errors{Retry: []model.RetryPolicy{{
		Name: "registry", Patterns: []string{"status code 503"}, MaxAttempts: 3,
	}}}
	attempts := 0
	_, ignore, err := retryRegistryResolution(t.Context(), policies, func() (string, error) {
		attempts++
		return "", errors.New("GET https://reg/acme: [status code\n503]")
	})
	require.Error(t, err)
	assert.Nil(t, ignore)
	assert.Equal(t, 3, attempts, "a pattern that matches the cleaned text retries up to max_attempts")
}

func TestMatchingRegistryIgnore(t *testing.T) {
	policies := []model.IgnorePolicy{
		{Name: "vetoed", Patterns: []string{"status code 503", "!acme"}},
		{Name: "first", Patterns: []string{"status code 503"}},
		{Name: "second", Patterns: []string{"status code"}},
	}
	got := matchingRegistryIgnore(policies, "GET https://reg/acme: \x1b[31mstatus code\n 503\x1b[0m")
	require.NotNil(t, got)
	assert.Equal(t, "first", got.Name, "text is cleaned like a source fetch and the first match decides")
	assert.Nil(t, matchingRegistryIgnore(policies, "connection refused"))
	assert.Nil(t, matchingRegistryIgnore(nil, "status code 503"))
}

func TestRegistrySourceResolvesAtPreparation(t *testing.T) {
	host, requests := registryServer(t)
	request := registryUnit(t, `source = "tfr://`+host+`/acme/vpc/aws//modules/nat?version=1.4.0"`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, "git::https://github.com/acme/terraform-aws-vpc.git//modules/nat?ref=v1.4.0", result.Source)
	assert.Empty(t, result.Folders, "a registry module is remote and has no local watch")
	assert.NotEmpty(t, result.WorkingDir)
	assert.Positive(t, requests.Load())
}

func TestRegistrySourceVersionSelection(t *testing.T) {
	host, _ := registryServer(t)
	latest, err := Resolve(t.Context(), registryUnit(t, `source = "tfr://`+host+`/acme/vpc/aws"`))
	require.NoError(t, err)
	assert.Equal(t, "git::https://github.com/acme/terraform-aws-vpc.git?ref=v2.0.0", latest.Source)

	constrained, err := Resolve(t.Context(), registryUnit(t, "source = \"tfr://"+host+"/acme/vpc/aws\"\nversion = \"~> 1.0\""))
	require.NoError(t, err)
	assert.Equal(t, "git::https://github.com/acme/terraform-aws-vpc.git?ref=v1.4.0", constrained.Source)
}

func TestRegistrySourceStaysUnresolvedWithoutNetwork(t *testing.T) {
	host, requests := registryServer(t)
	source := "tfr://" + host + "/acme/vpc/aws?version=1.0.0"
	for _, phase := range []string{"discover", "backend"} {
		request := registryUnit(t, `source = "`+source+`"`)
		request.Phase = phase
		if phase == "discover" {
			request.Command = "discover"
		}
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err, phase)
		assert.Equal(t, int32(0), requests.Load(), phase)
		if phase == "discover" {
			assert.Equal(t, source, result.Source)
			assert.Empty(t, result.Folders)
		}
	}
}

func TestRegistrySourceIsNotResolvedForAnExcludedOperation(t *testing.T) {
	host, requests := registryServer(t)
	source := "tfr://" + host + "/acme/vpc/aws?version=1.0.0"
	request := registryUnit(t, `source = "`+source+`"
}
exclude {
  if      = true
  actions = ["plan"]
  no_run  = true`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, int32(0), requests.Load(), "a prevented plan must not contact the registry")
	assert.Equal(t, source, result.Source)

	request.Command = "apply"
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	assert.Positive(t, requests.Load(), "an operation the block does not prevent resolves the source")
	assert.NotEqual(t, source, result.Source)
}

func TestRegistrySourceFailureIsAnActionableProblem(t *testing.T) {
	host, _ := registryServer(t)
	_, err := Resolve(t.Context(), registryUnit(t, `source = "tfr://`+host+`/acme/missing/aws?version=1.0.0"`))
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	assert.Equal(t, model.CodeInvalidSource, problem.Code)
	assert.Equal(t, "terraform.source", problem.Key)
	assert.NotEmpty(t, problem.Hints)
}

func TestNormalizeGetterURL(t *testing.T) {
	for source, want := range map[string]string{
		"github.com/acme/repo?ref=v1":             "git::https://github.com/acme/repo.git?ref=v1",
		"github.com/acme/repo.git//sub?ref=v1":    "git::https://github.com/acme/repo.git//sub?ref=v1",
		"bitbucket.org/acme/repo":                 "git::https://bitbucket.org/acme/repo.git",
		"github.com/acme/repo/modules/vpc?ref=v1": "git::https://github.com/acme/repo.git//modules/vpc?ref=v1",
		"git::https://example.test/repo.git":      "git::https://example.test/repo.git",
		"https://example.test/module.tar.gz":      "https://example.test/module.tar.gz",
	} {
		assert.Equal(t, want, normalizeGetterURL(source))
	}
}

func TestIsOpenTofu(t *testing.T) {
	assert.True(t, IsOpenTofu(filepath.Join("bin", "tofu")))
	assert.True(t, IsOpenTofu(`C:\tools\tofu.exe`))
	assert.False(t, IsOpenTofu("/usr/bin/terraform"))
	assert.False(t, IsOpenTofu(""))
}

// The host and the provider treat any executable whose name contains "tofu", in any case, as OpenTofu.
func TestIsOpenTofuUsesTheHostDetectionRule(t *testing.T) {
	for _, binary := range []string{"opentofu", "tofu-wrapper", "TOFU.exe", `C:\tools\OpenTofu.EXE`, "/opt/bin/tofu1.8"} {
		assert.True(t, IsOpenTofu(binary), binary)
	}
	for _, binary := range []string{"terraform", "/opt/tofu/bin/terraform", "terraform-wrapper.exe"} {
		assert.False(t, IsOpenTofu(binary), binary, "only the executable name decides, not its directories")
	}
}

func TestUnsupportedSourceTransportsStillFail(t *testing.T) {
	for _, source := range []string{"s3::https://bucket/module.zip", "ftp://example.test/module"} {
		_, err := Resolve(t.Context(), registryUnit(t, `source = "`+source+`"`))
		var problem *model.Problem
		require.ErrorAs(t, err, &problem, source)
		assert.Equal(t, model.CodeInvalidSource, problem.Code)
	}
}

func iamUnit(t *testing.T, iam string) model.Request {
	t.Helper()
	request := nativeUnit(t, iam)
	request.Tofu = "/bin/tofu"
	return request
}

func TestIAMSettingsAreReturnedAsData(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::123456789012:role/deploy"
iam_assume_role_session_name = "session-${get_env("SESSION", "local")}"
iam_assume_role_duration = 7200
iam_web_identity_token = "token-value"
inputs = { account = "kept" }`)
	request.Environment = map[string]string{"SESSION": "ci"}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, &model.IAM{Role: "arn:aws:iam::123456789012:role/deploy", SessionName: "session-ci", DurationSeconds: 7200, WebIdentityToken: "token-value"}, result.IAM)
	require.Equal(t, "kept", result.Inputs["account"], "iam settings do not disturb the rest of the unit")
}

func TestIAMWebIdentityTokenFileIsAnchoredAtTheUnit(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::123456789012:role/deploy"
iam_web_identity_token = "token.jwt"`)
	unitDir := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "token.jwt"), []byte("jwt"), 0o600))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	resolvedUnitDir, err := filepath.EvalSymlinks(unitDir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(resolvedUnitDir, "token.jwt"), result.IAM.WebIdentityToken, "a relative token file resolves against the unit directory")

	require.Equal(t, "literal-token", unitTokenFile("literal-token", unitDir), "a token that names no file is kept")
	absolute := filepath.Join(unitDir, "token.jwt")
	require.Equal(t, absolute, unitTokenFile(absolute, t.TempDir()))
	require.Empty(t, unitTokenFile("", unitDir))
}

func TestIAMRoleIsEvaluatedBeforeInputs(t *testing.T) {
	// A role named by a local that a function computes is a plain string once the unit is read.
	request := iamUnit(t, "locals { account = \"123456789012\" }\niam_role = \"arn:aws:iam::${local.account}:role/x\"")
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123456789012:role/x", result.IAM.Role)
}

func TestIAMProbeResolvesRoleWithoutEvaluatingInputs(t *testing.T) {
	request := iamUnit(t, `locals { account = "123456789012" }
iam_role = "arn:aws:iam::${local.account}:role/x"
inputs = { account = get_aws_account_id() }`)
	request.Phase = "iam"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123456789012:role/x", result.IAM.Role)
	require.Nil(t, result.Inputs)
}

func TestIAMProbeSkipsRoleForExcludedOperation(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::123456789012:role/x"
exclude {
  if = true
  actions = ["plan"]
}`)
	request.Phase = "iam"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Nil(t, result.IAM)
}

// An exclude block that reads a feature must see the unit's feature declarations during the IAM probe, with
// the declared default and with an override, or the probe fails for a unit that has no IAM problem.
func TestIAMProbeEvaluatesFeatureDependentExclude(t *testing.T) {
	unit := `feature "skip" {
  default = false
}
iam_role = "arn:aws:iam::123456789012:role/x"
exclude {
  if = feature.skip.value
  actions = ["plan"]
}`
	t.Run("default applies without an override", func(t *testing.T) {
		request := iamUnit(t, unit)
		request.Phase = "iam"
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.NotNil(t, result.IAM)
		assert.Equal(t, "arn:aws:iam::123456789012:role/x", result.IAM.Role)
	})
	t.Run("override excludes the operation", func(t *testing.T) {
		request := iamUnit(t, unit)
		request.Phase = "iam"
		request.Features = map[string]string{"skip": "true"}
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		assert.Nil(t, result.IAM, "an excluded operation resolves no role")
	})
}

func TestAWSFunctionUsesStagedSessionCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Authorization"), "Credential=ASIASESSION/")
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Account>222222222222</Account><Arn>arn:aws:sts::222222222222:assumed-role/x/session</Arn><UserId>session</UserId></GetCallerIdentityResult></GetCallerIdentityResponse>`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL_STS", server.URL)
	request := iamUnit(t, `iam_role = "arn:aws:iam::222222222222:role/x"
inputs = { account = get_aws_account_id() }`)
	request.Environment = map[string]string{"AWS_ACCESS_KEY_ID": "ASIASESSION", "AWS_SECRET_ACCESS_KEY": "session-secret", "AWS_SESSION_TOKEN": "session-token", "AWS_REGION": "us-east-1"}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "222222222222", result.Inputs["account"])
}

func TestIAMEnvironmentOverridesTheUnit(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::1:role/from-unit"
iam_assume_role_duration = 900`)
	request.Environment = map[string]string{"TG_IAM_ASSUME_ROLE": "arn:aws:iam::2:role/from-env", "TG_IAM_ASSUME_ROLE_DURATION": "1800", "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME": "legacy"}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::2:role/from-env", result.IAM.Role)
	require.Equal(t, int64(1800), result.IAM.DurationSeconds)
	require.Equal(t, "legacy", result.IAM.SessionName)

	request.Environment = map[string]string{"TG_IAM_ASSUME_ROLE_DURATION": "soon"}
	_, err = Resolve(t.Context(), request)
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "iam_assume_role_duration", problem.Key)
}

func TestIAMTokenWithoutRoleIsIgnoredWithAWarning(t *testing.T) {
	result, err := Resolve(t.Context(), iamUnit(t, `iam_web_identity_token = "orphan-token"`))
	require.NoError(t, err)
	require.Nil(t, result.IAM)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, warningIAMTokenWithoutRole, result.Warnings[0].Code)
	require.NotContains(t, result.Warnings[0].Message, "orphan-token")

	none, err := Resolve(t.Context(), iamUnit(t, ""))
	require.NoError(t, err)
	require.Nil(t, none.IAM)
	require.Empty(t, none.Warnings)
}

func TestIAMSettingsDoNotRunCommandsDuringDiscovery(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::1:role/x"`)
	request.Phase, request.Command = "discover", "discover"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Nil(t, result.IAM, "discovery does not resolve credentials or roles")
}

// A unit that a stack generates exists only in the generation overlay on a clean checkout, and the host
// probes IAM before the provider generates the stack.
func TestIAMProbeSeesUnitsThatTheirStackGenerates(t *testing.T) {
	root := stackProject(t, map[string]string{
		"live/terragrunt.stack.hcl": "unit \"app\" {\n  source = \"../catalog/units/app\"\n  path   = \"app\"\n  no_dot_terragrunt_stack = true\n}\n",
	})
	module, err := json.Marshal(filepath.ToSlash(filepath.Join(root, "modules", "unit")))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalog", "units", "app", "terragrunt.hcl"),
		[]byte("iam_role = \"arn:aws:iam::123456789012:role/generated\"\nterraform { source = "+string(module)+" }\n"), 0o644))
	request := model.Request{
		Phase: "iam", Command: "plan", BasePath: root, Environment: map[string]string{}, Tofu: "/bin/tofu",
		Path:           filepath.Join(root, "live", "app", "terragrunt.hcl"),
		GenerationRoot: filepath.Join(root, "live", "terragrunt.stack.hcl"),
	}

	result, err := Resolve(t.Context(), request)

	require.NoError(t, err)
	require.NotNil(t, result.IAM)
	require.Equal(t, "arn:aws:iam::123456789012:role/generated", result.IAM.Role)
	require.NoFileExists(t, request.Path, "the generated unit exists only in the invocation's overlay")
}

func TestIAMProbeOfAnUnknownUnitStillFails(t *testing.T) {
	request := iamUnit(t, `iam_role = "arn:aws:iam::123456789012:role/x"`)
	request.Phase = "iam"
	request.Path = filepath.Join(filepath.Dir(request.Path), "missing", "terragrunt.hcl")
	_, err := Resolve(t.Context(), request)
	require.Error(t, err, "without a generation root the unit must exist")
}

func TestRejectedResolvedSourceNamesBothSources(t *testing.T) {
	problem := &model.Problem{Code: model.CodeInvalidSource, Summary: "Unsupported module source transport", Detail: "The module source scheme \"ftp\" is not supported."}

	err := rejectedResolvedSource("tfr:///terraform-aws-modules/vpc/aws?version=5.0.0", "ftp://example.invalid/vpc.zip", problem)

	var annotated *model.Problem
	require.ErrorAs(t, err, &annotated)
	assert.Equal(t, model.CodeInvalidSource, annotated.Code)
	assert.Contains(t, annotated.Detail, "ftp\" is not supported")
	assert.Contains(t, annotated.Detail, "tfr:///terraform-aws-modules/vpc/aws?version=5.0.0")
	assert.Contains(t, annotated.Detail, "ftp://example.invalid/vpc.zip")
	assert.NotContains(t, problem.Detail, "resolved to", "the original problem is not modified")

	plain := errors.New("not a problem")
	assert.Same(t, plain, rejectedResolvedSource("tfr:///a/b/c", "x", plain))
}

func TestRejectedResolvedSourceOmitsURLCredentials(t *testing.T) {
	tests := []struct {
		name     string
		resolved string
		want     string
	}{
		{"user and token", "ftp://alice:s3cret@example.invalid/vpc.zip", "ftp://example.invalid/vpc.zip"},
		{"token only", "git::https://ghp_token@github.com/org/repo.git?ref=v1", "git::https://github.com/org/repo.git?ref=v1"},
		{"no credentials", "ftp://example.invalid/vpc.zip", "ftp://example.invalid/vpc.zip"},
		{"at sign in the path", "ftp://example.invalid/a@b.zip", "ftp://example.invalid/a@b.zip"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problem := &model.Problem{Code: model.CodeInvalidSource, Summary: "Unsupported", Detail: "Not supported.", Safe: true}
			err := rejectedResolvedSource("tfr:///a/b/c", test.resolved, problem)

			var annotated *model.Problem
			require.ErrorAs(t, err, &annotated)
			assert.Contains(t, annotated.Detail, test.want)
			assert.NotContains(t, annotated.Detail, "s3cret")
			assert.NotContains(t, annotated.Detail, "ghp_token")
			assert.NotContains(t, annotated.Detail, "alice")
		})
	}
}

// privateRegistry is a module registry that answers 401 to every request without the bearer token
// "hook-secret", and a unit that names a module in it.
type privateRegistry struct {
	request model.Request
	// source is the unit's source as written.
	source string
	// hostname is the registry host without its port, as a credentials block names it.
	hostname string
	// credentials is the CLI configuration file the unit's environment names. It does not exist
	// until login creates it.
	credentials string
	// requests counts the registry requests, and unauthorized those that carried no valid token.
	requests, unauthorized atomic.Int32
}

// login does what a read-config hook of the unit does in the host: it creates the CLI configuration
// file that holds the registry token.
func (r *privateRegistry) login(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(r.credentials, []byte(fmt.Sprintf("credentials %q {\n  token = \"hook-secret\"\n}\n", r.hostname)), 0o600))
}

const (
	registryLoginHook = `
  after_hook "registry_login" {
    commands = ["read-config"]
    execute  = ["registry-login"]
  }`
	privateModuleURL = "git::https://github.com/acme/terraform-aws-vpc.git?ref=v1.0.0"
)

// newPrivateRegistry starts the registry and writes a unit whose terraform block holds the registry
// source followed by body.
func newPrivateRegistry(t *testing.T, body string) *privateRegistry {
	t.Helper()
	registry := &privateRegistry{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		registry.requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer hook-secret" {
			registry.unauthorized.Add(1)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/.well-known/terraform.json" {
			fmt.Fprint(w, `{"modules.v1":"/v1/modules/"}`)
			return
		}
		w.Header().Set("X-Terraform-Get", "github.com/acme/terraform-aws-vpc?ref=v1.0.0")
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	previous := registryClient
	registryClient = server.Client()
	t.Cleanup(func() { registryClient = previous })
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	registry.hostname = address.Hostname()
	registry.source = "tfr://" + address.Host + "/acme/vpc/aws?version=1.0.0"
	registry.request = registryUnit(t, `source = "`+registry.source+`"`+body)
	registry.credentials = filepath.Join(filepath.Dir(registry.request.Path), "registry.tfrc")
	registry.request.Environment["TF_CLI_CONFIG_FILE"] = registry.credentials
	registry.request.Environment["TOFU_CLI_CONFIG_FILE"] = registry.credentials
	return registry
}

// A read-config after hook can create the credentials file of a private registry. The host runs that
// hook only once preparation has returned, so the first preparation must not contact the registry:
// the source stays as written, and the preparation that follows the hook resolves it.
func TestRegistrySourceWaitsForReadConfigHooks(t *testing.T) {
	for _, command := range []string{model.LifecycleReadConfig, model.LifecycleReadConfigAlias} {
		t.Run(command, func(t *testing.T) {
			registry := newPrivateRegistry(t, `
  after_hook "registry_login" {
    commands = ["`+command+`"]
    execute  = ["registry-login"]
  }`)

			first, err := Resolve(t.Context(), registry.request)
			require.NoError(t, err, "the registry must not be asked before the hook has created the credentials")
			assert.EqualValues(t, 0, registry.requests.Load(), "the first registry request must follow the read-config hook")
			assert.Equal(t, registry.source, first.Source, "the source stays as written until the hook has run")
			assert.Nil(t, first.RunIgnored)
			require.NotNil(t, first.Execution)
			require.Len(t, first.Execution.AfterHooks, 1, "the host receives the hook it has to run")
			assert.Equal(t, "registry_login", first.Execution.AfterHooks[0].Name)

			registry.login(t)
			registry.request.SourceHooksCompleted = true
			second, err := Resolve(t.Context(), registry.request)
			require.NoError(t, err)
			assert.Equal(t, privateModuleURL, second.Source)
			assert.Positive(t, registry.requests.Load())
			assert.EqualValues(t, 0, registry.unauthorized.Load(), "every registry request carries the token the hook supplied")
			assert.Equal(t, first.WorkingDir, second.WorkingDir, "the working directory is named after the source as written")
		})
	}
}

// Deferral is only for a registry source with a read-config after hook. A registry source without
// such a hook resolves in the single preparation, as before.
func TestRegistrySourceWithoutReadConfigHookResolvesAtOnce(t *testing.T) {
	for name, hooks := range map[string]string{
		"no hooks": "",
		"a plan hook": `
  after_hook "notify" {
    commands = ["plan"]
    execute  = ["notify"]
  }`,
		"an init-from-module hook": `
  after_hook "fetched" {
    commands = ["init-from-module"]
    execute  = ["fetched"]
  }`,
	} {
		t.Run(name, func(t *testing.T) {
			registry := newPrivateRegistry(t, hooks)
			registry.login(t)
			result, err := Resolve(t.Context(), registry.request)
			require.NoError(t, err)
			assert.Equal(t, privateModuleURL, result.Source)
			assert.Positive(t, registry.requests.Load())
			assert.EqualValues(t, 0, registry.unauthorized.Load())
		})
	}
}

// A read-config after hook changes nothing for a source that is not a registry source.
func TestReadConfigHookLeavesOtherSourcesUnchanged(t *testing.T) {
	for _, completed := range []bool{false, true} {
		registry := newPrivateRegistry(t, registryLoginHook)
		unit := "terraform {\n  source = \"" + privateModuleURL + "\"" + registryLoginHook + "\n}\n"
		require.NoError(t, os.WriteFile(registry.request.Path, []byte(unit), 0o644))
		registry.request.SourceHooksCompleted = completed
		result, err := Resolve(t.Context(), registry.request)
		require.NoError(t, err)
		assert.Equal(t, privateModuleURL, result.Source)
		assert.EqualValues(t, 0, registry.requests.Load())
	}
}

// The preparation that follows the hooks keeps the exclusion guard and the errors policies of a
// single preparation: a prevented operation never contacts the registry, an ignore policy ends the
// run, and a retry policy retries.
func TestDeferredRegistrySourceKeepsGuardAndPolicies(t *testing.T) {
	t.Run("excluded", func(t *testing.T) {
		registry := newPrivateRegistry(t, registryLoginHook+`
}
exclude {
  if      = true
  actions = ["plan"]
  no_run  = true`)
		registry.login(t)
		registry.request.SourceHooksCompleted = true
		result, err := Resolve(t.Context(), registry.request)
		require.NoError(t, err)
		assert.EqualValues(t, 0, registry.requests.Load(), "a prevented plan must not contact the registry")
		assert.Equal(t, registry.source, result.Source)
	})
	t.Run("ignored", func(t *testing.T) {
		registry := newPrivateRegistry(t, registryLoginHook+`
}
errors {
  ignore "private" {
    ignorable_errors = ["status code 401"]
    message = "no access"
  }`)
		first, err := Resolve(t.Context(), registry.request)
		require.NoError(t, err)
		assert.Nil(t, first.RunIgnored, "nothing failed yet: the registry was not asked")
		assert.EqualValues(t, 0, registry.requests.Load())

		// The hook did not create the credentials, so the registry rejects the request.
		registry.request.SourceHooksCompleted = true
		second, err := Resolve(t.Context(), registry.request)
		require.NoError(t, err)
		require.NotNil(t, second.RunIgnored)
		assert.Equal(t, model.IgnoredRun{Policy: "private", Message: "no access"}, *second.RunIgnored)
		assert.Equal(t, registry.source, second.Source)
		assert.EqualValues(t, 1, registry.requests.Load(), "an ignored failure is not retried")
	})
	t.Run("retried", func(t *testing.T) {
		registry := newPrivateRegistry(t, registryLoginHook+`
}
errors {
  retry "private" {
    retryable_errors = ["status code 401"]
    max_attempts = 3
    sleep_interval_sec = 0
  }`)
		registry.request.SourceHooksCompleted = true
		_, err := Resolve(t.Context(), registry.request)
		var problem *model.Problem
		require.ErrorAs(t, err, &problem)
		assert.Equal(t, model.CodeInvalidSource, problem.Code)
		assert.EqualValues(t, 3, registry.requests.Load(), "the failure is retried up to max_attempts")
		assert.EqualValues(t, 3, registry.unauthorized.Load())
	})
}

// A backend read never resolves a registry source, whether or not the hooks have run.
func TestDeferredRegistrySourceIsNotResolvedForBackendReads(t *testing.T) {
	registry := newPrivateRegistry(t, registryLoginHook)
	registry.login(t)
	registry.request.Phase, registry.request.SourceHooksCompleted = phaseBackend, true
	_, err := Resolve(t.Context(), registry.request)
	require.NoError(t, err)
	assert.EqualValues(t, 0, registry.requests.Load())
}
