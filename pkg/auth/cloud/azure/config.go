package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// cloudConfigurations maps CloudEnvironment names to the corresponding
// azcore/cloud.Configuration, mirroring the endpoints in cloudEnvironments.
var cloudConfigurations = map[string]cloud.Configuration{
	"public":       cloud.AzurePublic,
	"usgovernment": cloud.AzureGovernment,
	"china":        cloud.AzureChina,
}

// BuildAzureCredentialFromCreds wraps an already-acquired Atmos Azure access
// token as an azcore.TokenCredential, for constructing ARM SDK clients (AKS,
// ACR) from Atmos credentials the same way
// pkg/auth/cloud/aws.BuildAWSConfigFromCreds bridges AWS credentials to an
// aws.Config.
func BuildAzureCredentialFromCreds(creds types.ICredentials) (azcore.TokenCredential, error) {
	defer perf.Track(nil, "azure.BuildAzureCredentialFromCreds")()

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected Azure credentials", errUtils.ErrAuthenticationFailed)
	}

	return &types.StaticTokenCredential{
		Token: azcore.AccessToken{Token: azureCreds.AccessToken},
	}, nil
}

// BuildARMClientOptions maps a CloudEnvironment to the ARM SDK client options
// needed to target the correct sovereign cloud (public, usgovernment, china).
// A nil cloudEnv defaults to the public cloud.
func BuildARMClientOptions(cloudEnv *CloudEnvironment) *arm.ClientOptions {
	defer perf.Track(nil, "azure.BuildARMClientOptions")()

	name := "public"
	if cloudEnv != nil {
		name = cloudEnv.Name
	}

	cfg, ok := cloudConfigurations[name]
	if !ok {
		cfg = cloud.AzurePublic
	}

	return &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{Cloud: cfg},
	}
}
