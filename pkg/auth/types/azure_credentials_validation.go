package types

import (
	"errors"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

const (
	// The azureValidationTimeout bounds the Resource Manager call, like the AWS STS validation.
	azureValidationTimeout = 15 * time.Second
	// The azureValidationMaxRetries keeps an unreachable Resource Manager from retrying for long.
	azureValidationMaxRetries int32 = 1
)

// azureTokenRejectionCodes are Resource Manager error codes that reject the access token itself,
// as opposed to answers about the subscription (AuthorizationFailed, SubscriptionNotFound).
var azureTokenRejectionCodes = map[string]struct{}{
	"InvalidAuthenticationToken":         {},
	"InvalidAuthenticationTokenAudience": {},
	"InvalidAuthenticationTokenTenant":   {},
	"ExpiredAuthenticationToken":         {},
	"AuthenticationFailed":               {},
}

// azureValidationClouds maps AzureCredentials.CloudEnvironment to the Resource Manager cloud the
// credentials were minted for. Validating a sovereign-cloud token against the public cloud would
// be answered with a rejection although the token is valid.
var azureValidationClouds = map[string]cloud.Configuration{
	"usgovernment": cloud.AzureGovernment,
	"china":        cloud.AzureChina,
}

// armValidationOptions returns the Resource Manager client options Validate uses: the cloud of
// the credentials and a short retry budget, unless options were injected.
func (c *AzureCredentials) armValidationOptions() *arm.ClientOptions {
	if c.validationClientOptions != nil {
		return c.validationClientOptions
	}
	cloudConfig, ok := azureValidationClouds[c.CloudEnvironment]
	if !ok {
		cloudConfig = cloud.AzurePublic
	}
	return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Cloud: cloudConfig,
		Retry: policy.RetryOptions{MaxRetries: azureValidationMaxRetries},
	}}
}

// isAzureTokenRejection reports whether err shows that Resource Manager answered and rejected the
// access token: an HTTP 401, or a known token rejection code. Other answers (no access to the
// subscription, an unknown subscription, a server fault) and transport failures are not.
// The lastStatus argument is the HTTP status of the last answer (see azureStatusRecorder). It keeps
// a 401 recognizable when its body could not be read: the SDK then returns the read error instead
// of a ResponseError.
func isAzureTokenRejection(err error, lastStatus int) bool {
	if lastStatus == http.StatusUnauthorized {
		return true
	}
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}
	if respErr.StatusCode == http.StatusUnauthorized {
		return true
	}
	_, ok := azureTokenRejectionCodes[respErr.ErrorCode]
	return ok
}

// azureStatusRecorder is a per-retry pipeline policy that records the HTTP status of the last
// answer. It runs below the bearer token policy, which drops the response when reading the body
// fails, so the status survives even then. A try without a response resets it to 0.
type azureStatusRecorder struct {
	statusCode int
}

// Do records the status of the response the rest of the pipeline returned.
func (r *azureStatusRecorder) Do(req *policy.Request) (*http.Response, error) {
	resp, err := req.Next()
	r.statusCode = 0
	if resp != nil {
		r.statusCode = resp.StatusCode
	}
	return resp, err
}
