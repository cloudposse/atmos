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
func isAzureTokenRejection(err error) bool {
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
