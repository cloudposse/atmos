package engine

import (
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
		"github.com/acme/repo?ref=v1":          "git::https://github.com/acme/repo.git?ref=v1",
		"github.com/acme/repo.git//sub?ref=v1": "git::https://github.com/acme/repo.git//sub?ref=v1",
		"bitbucket.org/acme/repo":              "git::https://bitbucket.org/acme/repo.git",
		"git::https://example.test/repo.git":   "git::https://example.test/repo.git",
		"https://example.test/module.tar.gz":   "https://example.test/module.tar.gz",
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

func TestIAMRoleIsEvaluatedBeforeInputs(t *testing.T) {
	// A role named by a local that a function computes is a plain string once the unit is read.
	request := iamUnit(t, "locals { account = \"123456789012\" }\niam_role = \"arn:aws:iam::${local.account}:role/x\"")
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123456789012:role/x", result.IAM.Role)
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
