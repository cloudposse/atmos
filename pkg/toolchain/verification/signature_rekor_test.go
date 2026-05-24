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

	rekor503 := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: searching log query: [POST /api/v1/log/entries/retrieve][503] retrieveLogEntryDefault " +
		`{"code":503,"message":"service unavailable"}`

	tampered := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: invalid signature when validating ASN.1 encoded signature"

	identityMismatch := "cosign [verify-blob ...]: exit status 1\n" +
		"Error: none of the expected identities matched what was in the certificate"

	cases := []struct {
		name        string
		err         error
		wantWrapped bool
	}{
		{name: "nil error stays nil", err: nil, wantWrapped: false},
		{name: "rekor 400 searchLogQueryBadRequest is retryable", err: errors.New(rekor400), wantWrapped: true},
		{name: "rekor 503 on tlog retrieve endpoint is retryable", err: errors.New(rekor503), wantWrapped: true},
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
