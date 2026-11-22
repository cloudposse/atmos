package aws

// Tests for the device-token poller (sso_poll.go): cancellation, RFC 8628
// slow_down handling and error causes (issue #3720, finding 4.5).

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// errPollRequestAborted stands in for the transport error of a request whose context was cancelled.
var errPollRequestAborted = errors.New("request aborted")

var slowDownReply = fakeOIDCResponse{
	status:    http.StatusBadRequest,
	errorType: "SlowDownException",
	body:      `{"error":"slow_down"}`,
}

// pollTestInputs returns the registration and device-authorization outputs a poll needs.
func pollTestInputs(expiresIn, interval int32) (*ssooidc.RegisterClientOutput, *ssooidc.StartDeviceAuthorizationOutput) {
	return &ssooidc.RegisterClientOutput{ClientId: aws.String("client-id"), ClientSecret: aws.String("client-secret")},
		&ssooidc.StartDeviceAuthorizationOutput{DeviceCode: aws.String("device-code"), ExpiresIn: expiresIn, Interval: interval}
}

// recordingWait records each requested polling interval without sleeping.
type recordingWait struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (r *recordingWait) wait(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.waits = append(r.waits, d)
	r.mu.Unlock()
	return ctx.Err()
}

func newPollTestProvider() *ssoProvider {
	return &ssoProvider{name: "sso", startURL: "https://example.awsapps.com/start", region: "us-east-1"}
}

// TestPollForAccessToken_ContextCancellation calls the poller with a cancelled
// context and verifies it returns at once, before any CreateToken request,
// instead of sleeping through the polling interval.
func TestPollForAccessToken_ContextCancellation(t *testing.T) {
	srv := newFakeOIDCServer(t, pendingReply)
	p := newPollTestProvider()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	register, auth := pollTestInputs(600, 30)
	done := make(chan error, 1)
	go func() {
		_, err := p.pollForAccessToken(ctx, newFakeOIDCClient(srv.URL), register, auth)
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errUtils.ErrSSOTokenCreationFailed)
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("pollForAccessToken ignored context cancellation")
	}
	assert.Equal(t, int32(0), srv.calls.Load(), "no token request is made after cancellation")
}

// TestPollForAccessToken_CancelledWhilePending verifies that a cancellation
// that arrives between polls stops the poller with the context error.
func TestPollForAccessToken_CancelledWhilePending(t *testing.T) {
	srv := newFakeOIDCServer(t, pendingReply)
	p := newPollTestProvider()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits int
	p.pollWait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 2 {
			// The user aborts while the first poll is still pending.
			cancel()
		}
		return ctx.Err()
	}

	register, auth := pollTestInputs(600, 1)
	_, err := p.pollForAccessToken(ctx, newFakeOIDCClient(srv.URL), register, auth)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), srv.calls.Load())
}

// TestPollForAccessToken_RequestFailsAfterCancellation verifies that when the
// context is cancelled during a CreateToken request, the cancellation, not a
// generic token error, is reported.
func TestPollForAccessToken_RequestFailsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &mockTokenRefresher{createTokenFunc: func(context.Context, *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
		cancel()
		return nil, errPollRequestAborted
	}}
	p := newPollTestProvider()
	p.pollWait = (&recordingWait{}).wait

	register, auth := pollTestInputs(600, 1)
	_, err := p.pollForAccessToken(ctx, client, register, auth)
	require.ErrorIs(t, err, context.Canceled)
}

// TestPollForAccessToken_SlowDownAddsFiveSeconds verifies RFC 8628 section 3.5:
// every slow_down response adds five seconds to the interval for this and all
// later requests, and authorization_pending keeps the current interval.
func TestPollForAccessToken_SlowDownAddsFiveSeconds(t *testing.T) {
	srv := newFakeOIDCServer(t, slowDownReply, slowDownReply, pendingReply, tokenReply("access", "refresh"))
	p := newPollTestProvider()
	recorder := &recordingWait{}
	p.pollWait = recorder.wait

	register, auth := pollTestInputs(600, 1)
	result, err := p.pollForAccessToken(context.Background(), newFakeOIDCClient(srv.URL), register, auth)
	require.NoError(t, err)
	assert.Equal(t, "access", result.AccessToken)
	assert.Equal(t, int32(4), srv.calls.Load())
	assert.Equal(t, []time.Duration{
		1 * time.Second,  // Initial delay.
		6 * time.Second,  // First slow_down: 1s + 5s.
		11 * time.Second, // Second slow_down: 6s + 5s.
		11 * time.Second, // Pending keeps the slowed-down interval.
	}, recorder.waits)
}

// TestPollForAccessToken_TerminalErrorKeepsCause verifies that the AWS error
// that ended polling stays in the returned error chain.
func TestPollForAccessToken_TerminalErrorKeepsCause(t *testing.T) {
	srv := newFakeOIDCServer(t, accessDeniedReply)
	p := newPollTestProvider()
	p.pollWait = (&recordingWait{}).wait

	register, auth := pollTestInputs(600, 1)
	_, err := p.pollForAccessToken(context.Background(), newFakeOIDCClient(srv.URL), register, auth)
	require.ErrorIs(t, err, errUtils.ErrSSOTokenCreationFailed)
	var denied *ssooidctypes.AccessDeniedException
	assert.True(t, errors.As(err, &denied), "the AWS error must be kept as the cause: %v", err)
}

// TestWaitForPollInterval verifies the default wait returns after the interval
// and returns the context error when cancelled first.
func TestWaitForPollInterval(t *testing.T) {
	require.NoError(t, waitForPollInterval(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	require.ErrorIs(t, waitForPollInterval(ctx, time.Hour), context.Canceled)
	assert.Less(t, time.Since(start), time.Minute)
}
