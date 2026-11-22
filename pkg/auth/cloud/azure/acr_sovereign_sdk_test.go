package azure

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAADTransport is an in-memory stand-in for Azure AD: it answers the
// OpenID discovery and token requests MSAL sends and records every host asked.
// No request leaves the process.
type recordingAADTransport struct {
	mu    sync.Mutex
	hosts []string
	forms []string
}

func (r *recordingAADTransport) Do(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Host)
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		r.forms = append(r.forms, string(body))
	}
	r.mu.Unlock()

	authority := "https://" + req.URL.Host + "/tenant-123"
	var body string
	switch {
	case strings.HasSuffix(req.URL.Path, "/openid-configuration"):
		body = `{"token_endpoint":"` + authority + `/oauth2/v2.0/token","authorization_endpoint":"` + authority +
			`/oauth2/v2.0/authorize","issuer":"` + authority + `/v2.0"}`
	case strings.Contains(req.URL.Path, "/discovery/instance"):
		body = `{"tenant_discovery_endpoint":"` + authority + `/v2.0/.well-known/openid-configuration","metadata":[]}`
	default:
		body = `{"token_type":"Bearer","expires_in":3600,"access_token":"aad-token"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// TestLoadDefaultAzureCredentials_SDKUsesSovereignAuthority runs the real azidentity
// default credential chain (environment client-secret credential) against an
// in-memory Azure AD, and verifies the US Government options built for an
// azurecr.us registry send every AAD request to login.microsoftonline.us, for the
// US Government management scope.
func TestLoadDefaultAzureCredentials_SDKUsesSovereignAuthority(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "tenant-123")
	t.Setenv("AZURE_CLIENT_ID", "client-123")
	t.Setenv("AZURE_CLIENT_SECRET", "secret")
	t.Setenv("AZURE_AUTHORITY_HOST", "")
	t.Setenv("AZURE_TOKEN_CREDENTIALS", "EnvironmentCredential")

	transport := &recordingAADTransport{}
	orig := newDefaultAzureCredential
	t.Cleanup(func() { newDefaultAzureCredential = orig })
	newDefaultAzureCredential = func(options *azidentity.DefaultAzureCredentialOptions) (ambientTokenCredential, error) {
		require.NotNil(t, options, "a sovereign cloud must pass explicit options")
		withTransport := *options
		withTransport.Transport = transport
		withTransport.Retry = policy.RetryOptions{MaxRetries: -1}
		return orig(&withTransport)
	}

	cloudEnv, err := CloudEnvironmentForRegistry("myregistry.azurecr.us")
	require.NoError(t, err)
	creds, err := LoadDefaultAzureCredentials(t.Context(), cloudEnv)
	require.NoError(t, err)
	assert.Equal(t, "aad-token", creds.AccessToken)
	assert.Equal(t, "usgovernment", creds.CloudEnvironment)

	require.NotEmpty(t, transport.hosts)
	for _, host := range transport.hosts {
		assert.Equal(t, "login.microsoftonline.us", host, "every AAD request must go to the US Government authority")
	}
	assert.Contains(t, strings.Join(transport.forms, "\n"), "management.usgovcloudapi.net",
		"the token must be requested for the US Government ARM scope")
}
