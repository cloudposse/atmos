package okta

import (
	"io"
	"os"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// captureStdio redirects os.Stdout and os.Stderr while fn runs and returns what was written to each.
func captureStdio(t *testing.T, fn func()) (string, string) {
	t.Helper()

	drain := func(r *os.File) <-chan string {
		done := make(chan string, 1)
		go func() {
			data, _ := io.ReadAll(r)
			done <- string(data)
		}()
		return done
	}

	// Each pipe is released by a cleanup registered as soon as it opens, so a failure later in this
	// helper does not leak it. Closing an already closed end again only returns an error, which is ignored.
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = outW.Close()
		_ = outR.Close()
	})
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = errW.Close()
		_ = errR.Close()
	})
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := drain(outR), drain(errR)

	func() {
		defer func() { os.Stdout, os.Stderr = origOut, origErr }()
		fn()
	}()
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	stdout, stderr := <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return stdout, stderr
}

// Regression: the Duo U2F touch prompts were printed to stdout, the caller's data channel, which
// corrupts output such as `eval $(atmos auth env)`. They must go to stderr.
func TestChallengeDuoU2F_PromptsOnStderr(t *testing.T) {
	device := &mocks.U2FDevice{}
	device.On("Close").Return().Once()
	device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Once()
	device.On("Authenticate", mock.Anything).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sd"}, nil).Once()
	client := &DUOU2FClient{Device: device, ChallengeNonce: "nonce", AppID: "appID", KeyHandle: "kh", StateToken: "st"}

	var resp *ResponseData
	var err error
	stdout, stderr := captureStdio(t, func() { resp, err = client.ChallengeU2F() })

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "st", resp.SessionId)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "Touch the flashing U2F device to authenticate...")
	assert.Contains(t, stderr, "Touch accepted. Proceeding with authentication")
	device.AssertExpectations(t)
}
