package verification

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestClassifyCosignError(t *testing.T) {
	t.Parallel()

	// Realistic cosign stderr captured from a Sigstore Rekor flake in CI.
	rekor400 := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][400] searchLogQueryBadRequest " +
		`{"code":400,"message":"verifying signature: ecdsa: Invalid IEEE_P1363 encoded bytes"}`

	rekorStatusErr := func(status string) error {
		return errors.New("cosign [verify-blob ...]: exit status 1\n" +
			"Error: searching log query: [POST /api/v1/log/entries/retrieve][" + status + "] retrieveLogEntryDefault " +
			`{"code":` + status + `,"message":"service unavailable"}`)
	}

	// Realistic cosign stderr captured from an HTTP/2 transport flake in CI:
	// the connection to Rekor broke mid-request, so no verdict was rendered.
	rekorStreamInternalError := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: searching log query: stream error: stream ID 1; INTERNAL_ERROR; received from peer"

	transportErr := func(detail string) error {
		return errors.New("cosign [verify-blob ...]: exit status 1\n" +
			"Error: searching log query: Post \"https://rekor.sigstore.dev/api/v1/log/entries/retrieve\": " + detail)
	}

	tampered := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: invalid signature when validating ASN.1 encoded signature"

	identityMismatch := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: none of the expected identities matched what was in the certificate"

	// Rekor /retrieve with a non-5xx status that is also not in the explicit
	// 400-marker allowlist (e.g. 401) must surface immediately — only
	// allowlisted 5xx codes are retried on that endpoint.
	rekor401 := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][401] retrieveLogEntryUnauthorized"

	// Verbatim (trimmed) cosign output captured from cloudposse/atmos#2700 CI:
	// cosign fetches the --certificate URL itself (a GitHub release asset,
	// not the Rekor tlog endpoint) and the CDN returned a transient 504.
	certFetch504 := "cosign [verify-blob --certificate " +
		"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem ...]: exit status 1\n" +
		"Error: loading verifier from key opts: loading cert: loading URL " +
		"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem: server returned HTTP 504"

	sigFetch503 := "cosign [verify-blob --signature https://example.com/tool.sig ...]: exit status 1\n" +
		"Error: loading signature: loading URL https://example.com/tool.sig: server returned HTTP 503"

	// Captured from a concurrent toolchain install on macOS. cosign failed
	// while fetching the OpenTofu certificate sidecar, before it could make a
	// signature decision. This must use the bounded retry path.
	macOSCertificateFetchTLSFailure := "cosign [verify-blob --certificate " +
		"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_darwin_arm64.tar.gz.pem ...]: exit status 1\n" +
		"Error: loading verifier from key opts: loading cert: Get \"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_darwin_arm64.tar.gz.pem\": " +
		"tls: failed to verify certificate: SecPolicyCreateSSL error: 0"

	// A non-retryable status (e.g. 404, a genuinely missing asset) via the
	// same cosign HTTP-fetch code path must surface immediately.
	certFetch404 := "cosign [verify-blob --certificate https://example.com/missing.pem ...]: exit status 1\n" +
		"Error: loading verifier from key opts: loading cert: loading URL " +
		"https://example.com/missing.pem: server returned HTTP 404"

	cases := []struct {
		name        string
		err         error
		wantWrapped bool
	}{
		{name: "nil error stays nil", err: nil, wantWrapped: false},
		{name: "rekor 400 searchLogQueryBadRequest is retryable", err: errors.New(rekor400), wantWrapped: true},
		{name: "rekor 500 on tlog retrieve endpoint is retryable", err: rekorStatusErr("500"), wantWrapped: true},
		{name: "rekor 502 on tlog retrieve endpoint is retryable", err: rekorStatusErr("502"), wantWrapped: true},
		{name: "rekor 503 on tlog retrieve endpoint is retryable", err: rekorStatusErr("503"), wantWrapped: true},
		{name: "rekor 504 on tlog retrieve endpoint is retryable", err: rekorStatusErr("504"), wantWrapped: true},
		{name: "rekor stream internal error is retryable", err: errors.New(rekorStreamInternalError), wantWrapped: true},
		{name: "rekor 401 on tlog retrieve endpoint is NOT retryable", err: errors.New(rekor401), wantWrapped: false},
		{name: "cosign --certificate fetch 504 is retryable", err: errors.New(certFetch504), wantWrapped: true},
		{name: "cosign --signature fetch 503 is retryable", err: errors.New(sigFetch503), wantWrapped: true},
		{name: "macOS certificate-sidecar TLS failure is retryable", err: errors.New(macOSCertificateFetchTLSFailure), wantWrapped: true},
		{name: "cosign --certificate fetch 404 is NOT retryable", err: errors.New(certFetch404), wantWrapped: false},
		{name: "connection reset is retryable", err: transportErr("read tcp 10.0.0.1:443: connection reset by peer"), wantWrapped: true},
		{name: "TLS handshake timeout is retryable", err: transportErr("net/http: TLS handshake timeout"), wantWrapped: true},
		{name: "i/o timeout is retryable", err: transportErr("dial tcp 10.0.0.1:443: i/o timeout"), wantWrapped: true},
		{name: "unexpected EOF is retryable", err: transportErr("unexpected EOF"), wantWrapped: true},
		{name: "tampered artifact is NOT retryable", err: errors.New(tampered), wantWrapped: false},
		{name: "identity mismatch is NOT retryable", err: errors.New(identityMismatch), wantWrapped: false},
		{name: "generic cosign failure is NOT retryable", err: errors.New("cosign: exit status 1"), wantWrapped: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			classified := classifyCosignError(tc.err)
			if tc.err == nil {
				assert.NoError(t, classified)
				return
			}
			require.Error(t, classified)
			assert.Equal(t, tc.wantWrapped, errors.Is(classified, errUtils.ErrSignatureRetryable),
				"expected ErrSignatureRetryable wrap=%v, got err=%q", tc.wantWrapped, classified.Error())
			// Underlying cosign output must always be preserved in the chain.
			assert.ErrorContains(t, classified, tc.err.Error())
		})
	}
}

// flakyRunner returns retryableErr for the first failAttempts calls, then
// succeeds (returns nil) on the next call. After it succeeds, further calls
// also succeed. Tracks the total number of calls.
type flakyRunner struct {
	retryableErr  error
	failAttempts  int
	calls         int
	finalCallArgs []string
}

func (r *flakyRunner) Run(_ context.Context, _ string, args ...string) error {
	r.calls++
	r.finalCallArgs = append([]string(nil), args...)
	if r.calls <= r.failAttempts {
		return r.retryableErr
	}
	return nil
}

// fatalRunner always returns the supplied non-retryable error.
type fatalRunner struct {
	err   error
	calls int
}

func (r *fatalRunner) Run(_ context.Context, _ string, _ ...string) error {
	r.calls++
	return r.err
}

func TestRunCosignWithRetry_RecoversFromRekorFlake(t *testing.T) {
	t.Parallel()

	// Build an error that simulates the runner's wrapping. Need
	// classifyCosignError to detect the Rekor marker in the message.
	rekorErr := fmt.Errorf("%w: cosign [verify-blob ...]: exit status 1\n"+
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][400] searchLogQueryBadRequest",
		ErrSignatureFailed)

	runner := &flakyRunner{retryableErr: rekorErr, failAttempts: 2}
	req := &Request{Runner: runner}

	err := runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"})
	require.NoError(t, err)
	assert.Equal(t, 3, runner.calls, "expected 2 retried failures + 1 success")
	assert.Equal(t, []string{"verify-blob", "asset.tar.gz"}, runner.finalCallArgs)
}

func TestRunCosignWithRetry_RecoversFromMacOSCertificateTransportFailure(t *testing.T) {
	t.Parallel()

	retryableErr := fmt.Errorf("%w: cosign [verify-blob --certificate https://example.com/tool.pem ...]: exit status 1\n"+
		"Error: loading verifier from key opts: loading cert: Get \"https://example.com/tool.pem\": "+
		"tls: failed to verify certificate: SecPolicyCreateSSL error: 0", ErrSignatureFailed)
	runner := &flakyRunner{retryableErr: retryableErr, failAttempts: 1}
	req := &Request{Runner: runner}

	require.NoError(t, runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"}))
	assert.Equal(t, 2, runner.calls)
}

// TestRunCosignWithRetry_RecoversFromCertificateFetch504 reproduces the CI
// failure in cloudposse/atmos#2700: cosign fetches its --certificate URL
// directly, and a transient CDN 504 from that fetch must be retried the same
// as any other transient toolchain download failure.
func TestRunCosignWithRetry_RecoversFromCertificateFetch504(t *testing.T) {
	t.Parallel()

	certFetchErr := fmt.Errorf("%w: cosign [verify-blob --certificate "+
		"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem ...]: exit status 1\n"+
		"Error: loading verifier from key opts: loading cert: loading URL "+
		"https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem: server returned HTTP 504",
		ErrSignatureFailed)

	runner := &flakyRunner{retryableErr: certFetchErr, failAttempts: 2}
	req := &Request{Runner: runner}

	err := runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"})
	require.NoError(t, err)
	assert.Equal(t, 3, runner.calls, "expected 2 retried failures + 1 success")
	assert.Equal(t, []string{"verify-blob", "asset.tar.gz"}, runner.finalCallArgs)
}

func TestRunCosignWithRetry_RecoversFromTransportFlake(t *testing.T) {
	t.Parallel()

	// The exact HTTP/2 transport error observed in CI: the connection to
	// Rekor broke mid-request, so no signature verdict was rendered.
	streamErr := fmt.Errorf("%w: cosign [verify-blob ...]: exit status 1\n"+
		"Error: searching log query: stream error: stream ID 1; INTERNAL_ERROR; received from peer",
		ErrSignatureFailed)

	runner := &flakyRunner{retryableErr: streamErr, failAttempts: 2}
	req := &Request{Runner: runner}

	err := runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"})
	require.NoError(t, err)
	assert.Equal(t, 3, runner.calls, "expected 2 retried failures + 1 success")
	assert.Equal(t, []string{"verify-blob", "asset.tar.gz"}, runner.finalCallArgs)
}

func TestRunCosignWithRetry_DoesNotRetryRealFailures(t *testing.T) {
	t.Parallel()

	// A real "tampered artifact" failure must surface immediately, never
	// retried. Otherwise we'd mask tampering events.
	tamperErr := fmt.Errorf("%w: cosign [verify-blob ...]: exit status 1\n"+
		"Error: invalid signature when validating ASN.1 encoded signature",
		ErrSignatureFailed)

	runner := &fatalRunner{err: tamperErr}
	req := &Request{Runner: runner}

	err := runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"})
	require.Error(t, err)
	assert.Equal(t, 1, runner.calls, "tampering must not be retried")
	assert.False(t, errors.Is(err, errUtils.ErrSignatureRetryable),
		"real signature failures must not be classified as retryable")
	assert.ErrorIs(t, err, ErrSignatureFailed)
}

func TestRunCosignWithRetry_ExhaustsRetriesOnPersistentRekorOutage(t *testing.T) {
	t.Parallel()

	rekorErr := fmt.Errorf("%w: cosign [verify-blob ...]: exit status 1\n"+
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][400] searchLogQueryBadRequest",
		ErrSignatureFailed)

	runner := &fatalRunner{err: rekorErr}
	req := &Request{Runner: runner}

	err := runCosignWithRetry(context.Background(), req, []string{"verify-blob", "asset.tar.gz"})
	require.Error(t, err)
	assert.Equal(t, cosignRetryMaxAttempts, runner.calls,
		"should retry exactly cosignRetryMaxAttempts times before giving up")
	assert.ErrorIs(t, err, ErrSignatureFailed,
		"underlying signature failure must be preserved when retries exhaust")
	assert.Contains(t, err.Error(), "max attempts",
		"retry exhaustion message should call out the attempt budget")
}

// TestVerifyCosignRetriesViaPublicAPI ensures the retry path is actually
// wired into the public Verify entrypoint, not just into runCosignWithRetry.
func TestVerifyCosignRetriesViaPublicAPI(t *testing.T) {
	t.Parallel()

	rekorErr := fmt.Errorf("%w: cosign [verify-blob ...]: exit status 1\n"+
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][400] searchLogQueryBadRequest",
		ErrSignatureFailed)

	runner := &flakyRunner{retryableErr: rekorErr, failAttempts: 1}

	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Opts: []string{
					"--signature", "https://example.com/{{.Asset}}.sig",
					"--certificate-identity",
					"https://github.com/owner/tool/.github/workflows/release.yaml@refs/tags/{{.Version}}",
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, runner.calls, "expected 1 retried failure + 1 success")
	assert.Contains(t, result.SignatureMethods, "cosign")
}
