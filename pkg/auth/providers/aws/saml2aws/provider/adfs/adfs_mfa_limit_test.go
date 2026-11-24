package adfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// Regression: a server that keeps returning the one-time-code page made Authenticate prompt and
// submit forever. It must give up after maxMFAPromptAttempts codes.
func TestAuthenticateMFAPromptAttemptLimit(t *testing.T) {
	// The expectations are independent of the production constant: five codes are allowed, so one login
	// POST plus five code POSTs; the configured token is the first code, so four are prompted for.
	const (
		wantCodes   = 5
		wantPrompts = 4
	)
	pr := mocks.NewMockPrompter(gomock.NewController(t))
	// The controller verifies the exact prompt count when the test ends.
	pr.EXPECT().RequestSecurityCode("000000").Return("999999", nil).Times(wantPrompts)
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

	posts := fake.postedForms()
	require.Len(t, posts, 1+wantCodes)
	assert.Equal(t, []string{"111111"}, posts[1]["VerificationCode"])
	assert.Equal(t, []string{"999999"}, posts[wantCodes]["VerificationCode"])
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
			// One login POST plus 300 polls (a test-local limit, independent of the production constant).
			assert.Len(t, fake.postedForms(), 301)
		})
	}
}

// The limit only stops a server that never finishes: approval within the limit still succeeds.
func TestAuthenticateMFAPromptWithinLimit(t *testing.T) {
	pr := mocks.NewMockPrompter(gomock.NewController(t))
	pr.EXPECT().RequestSecurityCode("000000").Return("999999", nil).AnyTimes()
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
