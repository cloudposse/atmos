package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeSTSResponse is a canned AssumeRoleWithSAML response.
const fakeSTSResponse = `<AssumeRoleWithSAMLResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithSAMLResult><Credentials>` + // gitleaks:allow -- Fake STS response for a unit test.
	`<AccessKeyId>AKIAEXAMPLE</AccessKeyId><SecretAccessKey>fake-secret</SecretAccessKey><SessionToken>fake-session</SessionToken>` +
	`<Expiration>2030-01-01T00:00:00Z</Expiration></Credentials></AssumeRoleWithSAMLResult></AssumeRoleWithSAMLResponse>`

var errStubClient = errors.New("stub client failure")

// captureNewSAMLClient replaces newSAMLClient and records the config it was given.
func captureNewSAMLClient(t *testing.T, client saml2aws.SAMLClient, err error) **cfg.IDPAccount {
	t.Helper()
	original := newSAMLClient
	t.Cleanup(func() { newSAMLClient = original })
	var got *cfg.IDPAccount
	newSAMLClient = func(account *cfg.IDPAccount) (saml2aws.SAMLClient, error) {
		got = account
		return client, err
	}
	return &got
}

// newAuthTestProvider builds a provider with a non-browser driver so no Playwright setup runs.
func newAuthTestProvider(t *testing.T, spec map[string]any) *samlProvider {
	t.Helper()
	pAny, err := NewSAMLProvider("idp", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://idp.example.com/saml",
		Region: "us-east-1",
		Driver: "Okta",
		Spec:   spec,
	})
	require.NoError(t, err)
	p, ok := pAny.(*samlProvider)
	require.True(t, ok)
	p.RoleToAssumeFromAssertion = "AWS-Admin-CloudOPSNonProd"
	return p
}

func validAssertionB64(t *testing.T) string {
	t.Helper()
	assertion, err := os.ReadFile(filepath.Join("saml2aws", "testdata", "assertion.xml"))
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(assertion)
}

func TestSAMLProviderAuthenticateFailures(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(p *samlProvider)
		client     saml2aws.SAMLClient
		clientErr  error
		wantErr    error
		wantClient bool
	}{
		{
			name:    "unsupported driver stops before any client is created",
			mutate:  func(p *samlProvider) { p.config.Driver = "NoSuchDriver" },
			wantErr: errUtils.ErrInvalidProviderConfig,
		},
		{
			name:    "missing role hint",
			mutate:  func(p *samlProvider) { p.RoleToAssumeFromAssertion = "" },
			wantErr: errUtils.ErrInvalidAuthConfig,
		},
		{
			name:       "client creation fails",
			clientErr:  errStubClient,
			wantErr:    errUtils.ErrInvalidProviderConfig,
			wantClient: true,
		},
		{
			name:       "idp authentication fails",
			client:     stubSAMLClient{err: errStubClient},
			wantErr:    errUtils.ErrAuthenticationFailed,
			wantClient: true,
		},
		{
			name:       "empty assertion",
			client:     stubSAMLClient{},
			wantErr:    errUtils.ErrAuthenticationFailed,
			wantClient: true,
		},
		{
			name:       "assertion is not base64",
			client:     stubSAMLClient{assertion: "%%%not-base64%%%"},
			wantErr:    errUtils.ErrAwsSAMLDecodeFailed,
			wantClient: true,
		},
		{
			name:       "assertion has no AWS roles",
			client:     stubSAMLClient{assertion: base64.StdEncoding.EncodeToString([]byte(`<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol"></Response>`))},
			wantErr:    errUtils.ErrAwsSAMLDecodeFailed,
			wantClient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newAuthTestProvider(t, nil)
			if tt.mutate != nil {
				tt.mutate(p)
			}
			got := captureNewSAMLClient(t, tt.client, tt.clientErr)

			creds, err := p.Authenticate(context.Background())
			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, creds)
			if tt.wantClient {
				require.NotNil(t, *got, "the client must have been created")
				assert.Equal(t, "Okta", (*got).Provider)
			} else {
				assert.Nil(t, *got, "no client may be created")
			}
		})
	}
}

// The full flow: client, assertion decode, role selection by hint and STS exchange against a local endpoint.
func TestSAMLProviderAuthenticateSuccess(t *testing.T) {
	var gotForm string
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.PostForm.Get("RoleArn")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(fakeSTSResponse))
	}))
	defer sts.Close()

	p := newAuthTestProvider(t, map[string]any{"endpoint_url": sts.URL})
	captureNewSAMLClient(t, stubSAMLClient{assertion: validAssertionB64(t)}, nil)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")     // gitleaks:allow -- Fake credential for a unit test.
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test") // gitleaks:allow -- Fake credential for a unit test.

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAEXAMPLE", awsCreds.AccessKeyID)
	assert.Equal(t, "fake-session", awsCreds.SessionToken)
	assert.Equal(t, "us-east-1", awsCreds.Region)
	assert.Equal(t, "arn:aws:iam::123123123123:role/AWS-Admin-CloudOPSNonProd", gotForm, "the role matching the hint is assumed")
}

// A failing STS exchange is reported as an authentication failure.
func TestSAMLProviderAuthenticateSTSFailure(t *testing.T) {
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>AccessDenied</Code><Message>denied</Message></Error></ErrorResponse>`))
	}))
	defer sts.Close()

	p := newAuthTestProvider(t, map[string]any{"endpoint_url": sts.URL})
	captureNewSAMLClient(t, stubSAMLClient{assertion: validAssertionB64(t)}, nil)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")     // gitleaks:allow -- Fake credential for a unit test.
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test") // gitleaks:allow -- Fake credential for a unit test.

	creds, err := p.Authenticate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Nil(t, creds)
}
