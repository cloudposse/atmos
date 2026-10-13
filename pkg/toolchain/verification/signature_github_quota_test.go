package verification

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// attestationQuotaRunner lets tests inspect the temporary evidence before cleanup.
type attestationQuotaRunner func(context.Context, string, ...string) error

func (r attestationQuotaRunner) Run(ctx context.Context, name string, args ...string) error {
	return r(ctx, name, args...)
}

func TestVerifyGitHubAttestationInstallationQuota(t *testing.T) {
	t.Parallel()
	const repo = "terraform-linters/tflint"
	const workflow = repo + "/.github/workflows/release.yml"
	const predicate = "https://slsa.dev/provenance/v1"
	quotaErr := fmt.Errorf("%w: HTTP 403: API rate limit exceeded for installation (https://api.github.com/repos/%s/attestations/sha256:abc)", ErrSignatureFailed, repo)
	for _, tc := range []struct {
		name, response  string
		verificationErr error
		wantSuccess     bool
		wantUnavailable bool
	}{
		{name: "verified evidence", response: `{"attestations":[{"bundle":{"proof":1}},{"bundle":{"proof":2}}]}`, wantSuccess: true},
		{name: "wrong signer", response: `{"attestations":[{"bundle":{"proof":1}}]}`, verificationErr: fmt.Errorf("%w: certificate identity mismatch", ErrSignatureFailed)},
		{name: "verification transport error", response: `{"attestations":[{"bundle":{"proof":1}}]}`, verificationErr: fmt.Errorf("%w: HTTP 404 retrieving trust roots", ErrSignatureFailed)},
		{name: "tampered evidence", response: `{"attestations":[{"bundle":{"proof":1}}]}`, verificationErr: ErrSignatureFailed},
		{name: "missing evidence", response: `{"attestations":[]}`, wantUnavailable: true},
		{name: "malformed response", response: `invalid`},
		{name: "null response", response: `null`},
		{name: "missing array", response: `{}`},
		{name: "null array", response: `{"attestations":null}`},
		{name: "object instead of array", response: `{"attestations":{}}`},
		{name: "null bundle", response: `{"attestations":[{"bundle":null}]}`},
		{name: "truncated response", response: `{"attestations":[` + strings.Repeat(`{"bundle":{"proof":1}},`, 29) + `{"bundle":{"proof":1}}]}`},
		{name: "missing bundle", response: `{"attestations":[{}]}`},
		{name: "public download failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := []byte("signed artifact")
			asset := writeAsset(t, data)
			digest := sha256.Sum256(data)
			endpoint := fmt.Sprintf("https://api.github.com/repos/%s/attestations/sha256:%x?per_page=30", repo, digest)
			downloader := fakeDownloader{}
			if tc.response != "" {
				downloader[endpoint] = []byte(tc.response)
			}
			var calls int
			var bundlePath string
			originalArgs := []string{"attestation", "verify", asset, "--repo", repo, "--signer-workflow", workflow, "--predicate-type", predicate}
			runner := attestationQuotaRunner(func(_ context.Context, name string, args ...string) error {
				calls++
				assert.Equal(t, "gh", name)
				if calls == 1 {
					assert.Equal(t, originalArgs, args)
					return quotaErr
				}
				require.Len(t, args, len(originalArgs)+2)
				assert.Equal(t, originalArgs, args[:len(originalArgs)])
				assert.Equal(t, "--bundle", args[len(originalArgs)])
				bundlePath = args[len(args)-1]
				contents, err := os.ReadFile(bundlePath)
				require.NoError(t, err)
				assert.Contains(t, string(contents), "{\"proof\":1}\n")
				if tc.wantSuccess {
					assert.Equal(t, "{\"proof\":1}\n{\"proof\":2}\n", string(contents))
				}
				return tc.verificationErr
			})
			result, err := (&Verifier{publicAttestationDownloader: downloader}).Verify(context.Background(), Request{
				Tool:      &registry.Tool{RepoOwner: "terraform-linters", RepoName: "tflint", GitHubArtifactAttestations: registry.GitHubArtifactAttestations{SignerWorkflow: workflow, PredicateType: predicate}},
				AssetPath: asset, Runner: runner, Downloader: downloader,
				// A quota fallback failure must fail even under the default permissive availability policy.
				Policy: Policy{Checksums: PolicyDisabled, Signatures: PolicyWhenAvailable},
			})
			switch {
			case tc.wantSuccess:
				require.NoError(t, err)
				assert.Contains(t, result.SignatureMethods, "github_artifact_attestations")
			case tc.wantUnavailable:
				require.NoError(t, err)
				assert.NotContains(t, result.SignatureMethods, "github_artifact_attestations")
				assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "github artifact attestations unavailable"))
			default:
				require.ErrorIs(t, err, ErrSignatureFailed)
			}
			if bundlePath != "" {
				assert.NoFileExists(t, bundlePath)
			}
		})
	}
}

func TestGitHubAttestationQuotaDoesNotMatchVerificationFailures(t *testing.T) {
	t.Parallel()
	for _, message := range []string{
		"HTTP 403: forbidden", "HTTP 401: unauthorized", "invalid signature", "certificate identity mismatch",
		"HTTP 403: API rate limit exceeded for installation (https://enterprise.example/repos/org/repo)",
	} {
		assert.False(t, isGitHubAttestationInstallationQuota(fmt.Errorf("%w: %s", ErrSignatureFailed, message)))
	}
	assert.False(t, isGitHubAttestationInstallationQuota(nil))
}

func TestPublicAttestationPreservesMissingPredicatePolicy(t *testing.T) {
	t.Parallel()
	const releaseEvidence = `{"attestations":[{"bundle":{"dsseEnvelope":{"payloadType":"application/vnd.in-toto+json","payload":"eyJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9pbi10b3RvLmlvL2F0dGVzdGF0aW9uL3JlbGVhc2UvdjAuMiJ9"}}}]}`
	for _, policy := range []string{PolicyWhenAvailable, PolicyRequired} {
		t.Run(policy, func(t *testing.T) {
			t.Parallel()
			asset := writeAsset(t, []byte("artifact"))
			digest, err := digestFile(asset, "sha256")
			require.NoError(t, err)
			endpoint := "https://api.github.com/repos/terraform-linters/tflint/attestations/sha256:" + digest + "?per_page=30"
			var calls int
			runner := attestationQuotaRunner(func(_ context.Context, name string, args ...string) error {
				if name == "cosign" {
					return nil
				}
				calls++
				if calls == 1 {
					return fmt.Errorf("%w: HTTP 403: API rate limit exceeded for installation (%s)", ErrSignatureFailed, endpoint)
				}
				assert.NotContains(t, args, "--predicate-type", "gh must keep its default SLSA/v1 policy")
				contents, readErr := os.ReadFile(args[len(args)-1])
				require.NoError(t, readErr)
				assert.Contains(t, string(contents), "dsseEnvelope")
				// Actual gh verdict for TFLint0.59.1's release/v0.2 bundle under the unchanged SLSA/v1 policy.
				return fmt.Errorf("%w: Error: no attestations found with predicate type: https://slsa.dev/provenance/v1", ErrSignatureFailed)
			})
			result, err := (&Verifier{publicAttestationDownloader: fakeDownloader{endpoint: []byte(releaseEvidence)}}).Verify(context.Background(), Request{
				Tool:      &registry.Tool{RepoOwner: "terraform-linters", RepoName: "tflint", Cosign: registry.CosignConfig{Opts: []string{"--key", "public.pem"}}, GitHubArtifactAttestations: registry.GitHubArtifactAttestations{SignerWorkflow: "terraform-linters/tflint/.github/workflows/release.yml"}},
				AssetPath: asset, Runner: runner, Policy: Policy{Checksums: PolicyDisabled, Signatures: policy},
			})
			if policy == PolicyRequired {
				require.ErrorIs(t, err, ErrSignatureRequired)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"cosign"}, result.SignatureMethods)
			assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "github artifact attestations unavailable"))
			assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "no attestations found with predicate type:"))
		})
	}
}

type attestationTransport func(*http.Request) (*http.Response, error)

func (f attestationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAnonymousAttestationDownloader(t *testing.T) {
	t.Setenv("GH_TOKEN", "must-not-be-sent")
	t.Setenv("GITHUB_TOKEN", "also-must-not-be-sent")
	const endpoint = "https://api.github.com/repos/owner/tool/attestations/sha256:abc"
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		link      string
		wantError bool
	}{
		{"success", http.StatusOK, `{"attestations":[]}`, "", false},
		{"missing", http.StatusNotFound, "", "", true},
		{"redirect", http.StatusFound, "", "", true},
		{"oversized", http.StatusOK, strings.Repeat("x", (8<<20)+1), "", true},
		{"later page", http.StatusOK, `{"attestations":[]}`, `<https://api.github.com/next>; rel="next"`, true},
		{"last page", http.StatusOK, `{"attestations":[]}`, `<https://api.github.com/last>; rel="last"`, true},
		{"deprecation link", http.StatusOK, `{"attestations":[]}`, `<https://docs.github.com/en/rest/about-the-rest-api/api-versions>; rel="deprecation"; type="text/html"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			downloader := anonymousAttestationDownloader{transport: attestationTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, endpoint, req.URL.String())
				assert.Empty(t, req.Header.Get("Authorization"))
				assert.Empty(t, req.Header.Get("Cookie"))
				_, hasDeadline := req.Context().Deadline()
				assert.True(t, hasDeadline, "anonymous request must have a finite deadline")
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://other.example/bundle"}, "Link": []string{tc.link}}, Request: req}, nil
			})}
			body, err := downloader.Download(context.Background(), endpoint)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(body))
			}
			assert.Equal(t, 1, calls, "redirects must not trigger additional requests")
		})
	}
	for _, endpoint := range []string{"http://api.github.com/x", "https://other.example/x", "https://token@api.github.com/x", ":bad"} {
		_, err := (anonymousAttestationDownloader{}).Download(context.Background(), endpoint)
		require.Error(t, err)
	}
}
