package adfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// Regression: a server that keeps returning the one-time-code page made Authenticate prompt and
// submit forever. It must give up after maxMFAPromptAttempts codes.
func TestAuthenticateMFAPromptAttemptLimit(t *testing.T) {
	pr := &mocks.Prompter{}
	pr.On("RequestSecurityCode", "000000").Return("999999")
	usePrompter(t, pr)

	fake := &fakeADFS{
		loginHTML:  loginPage("/adfs/ls/"),
		responses:  []string{mfaPage(`<input name="VerificationCode" type="text" value="" />`)},
		repeatLast: true,
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "111111"))
	require.ErrorIs(t, err, ErrMFAAttemptsExceeded)
	assert.Empty(t, assertion)

	// One login POST plus one POST per allowed code; the configured token counts as the first code.
	posts := fake.postedForms()
	require.Len(t, posts, 1+maxMFAPromptAttempts)
	assert.Equal(t, []string{"111111"}, posts[1]["VerificationCode"])
	assert.Equal(t, []string{"999999"}, posts[maxMFAPromptAttempts]["VerificationCode"])
	pr.AssertNumberOfCalls(t, "RequestSecurityCode", maxMFAPromptAttempts-1)
}

// Regression: a server that keeps returning the Azure MFA wait page was polled forever.
func TestAuthenticateAzureMFAPollLimit(t *testing.T) {
	for _, method := range []string{"AzureMfaAuthentication", "AzureMfaServerAuthentication"} {
		t.Run(method, func(t *testing.T) {
			useFastAzurePolling(t)
			captureLog(t)
			fake := &fakeADFS{
				loginHTML:  loginPage("/adfs/ls/"),
				responses:  []string{azurePage(method, `<p id="instructions">Approve the sign-in request.</p>`)},
				repeatLast: true,
			}
			ts := newTestServer(t, fake)

			assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
			require.ErrorIs(t, err, ErrMFAAttemptsExceeded)
			assert.Empty(t, assertion)
			// One login POST plus one POST per allowed poll.
			assert.Len(t, fake.postedForms(), 1+maxAzureMFAPolls)
		})
	}
}

// The limit only stops a server that never finishes: approval within the limit still succeeds.
func TestAuthenticateMFAPromptWithinLimit(t *testing.T) {
	pr := &mocks.Prompter{}
	pr.On("RequestSecurityCode", "000000").Return("999999")
	usePrompter(t, pr)

	verification := mfaPage(`<input name="VerificationCode" type="text" value="" />`)
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{verification, verification, verification, verification, verification, samlResponsePage},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "111111"))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)
}
