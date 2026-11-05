package auth

import (
	"context"
	"errors"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	awsIdentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWSMaterializer reuses the built-in AWS profile and auth-context implementation.
func AWSMaterializer(name string, config *schema.Identity) Materializer {
	defer perf.Track(nil, "auth.AWSMaterializer")()

	provider := config.Provider
	if config.Via != nil && config.Via.Provider != "" {
		provider = config.Via.Provider
	}
	return &awsMaterializer{name: name, provider: provider}
}

type awsMaterializer struct{ name, provider, realm, region string }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (a *awsMaterializer) SetRealm(realm string) { a.realm = realm }

func (a *awsMaterializer) Apply(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "auth.awsMaterializer.Apply")()

	if err := ctx.Err(); err != nil {
		return err
	}
	if params == nil {
		return errUtils.ErrInvalidAuthConfig
	}
	credentials, ok := params.Credentials.(*types.AWSCredentials)
	if !ok {
		return errUtils.ErrInvalidAuthConfig
	}
	a.provider, a.realm, a.region = params.ProviderName, params.Realm, credentials.Region
	if err := awsCloud.SetupFiles(a.provider, a.name, credentials, "", a.realm); err != nil {
		return err
	}
	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext: params.AuthContext, StackInfo: params.StackInfo, ProviderName: a.provider,
		IdentityName: a.name, Credentials: credentials, Manager: params.Manager, Realm: a.realm,
	}); err != nil {
		return err
	}
	return awsCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo)
}

func (a *awsMaterializer) Environment() (map[string]string, error) {
	defer perf.Track(nil, "auth.awsMaterializer.Environment")()

	manager, err := awsCloud.NewAWSFileManager("", a.realm)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, variable := range manager.GetEnvironmentVariables(a.provider, a.name) {
		result[variable.Key] = variable.Value
	}
	if a.region != "" {
		result["AWS_REGION"], result["AWS_DEFAULT_REGION"] = a.region, a.region
	}
	return maps.Clone(result), nil
}

func (a *awsMaterializer) Paths() ([]types.Path, error) {
	defer perf.Track(nil, "auth.awsMaterializer.Paths")()

	manager, err := awsCloud.NewAWSFileManager("", a.realm)
	if err != nil {
		return nil, err
	}
	return []types.Path{
		{Location: manager.GetCredentialsPath(a.provider), Type: types.PathTypeFile, Purpose: "AWS credentials"},
		{Location: manager.GetConfigPath(a.provider), Type: types.PathTypeFile, Purpose: "AWS configuration"},
	}, nil
}

func (a *awsMaterializer) Logout(ctx context.Context) error {
	defer perf.Track(nil, "auth.awsMaterializer.Logout")()

	if a.provider == "" {
		return nil
	}
	manager, err := awsCloud.NewAWSFileManager("", a.realm)
	if err != nil {
		return err
	}
	return errors.Join(manager.RemoveCredentialsProfile(ctx, a.provider, a.name), manager.RemoveConfigProfile(ctx, a.provider, a.name))
}

func (a *awsMaterializer) Load(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "auth.awsMaterializer.Load")()

	environment, err := a.Environment()
	if err != nil {
		return nil, err
	}
	return awsIdentity.LoadCredentialsFromEnvironment(ctx, environment)
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (a *awsMaterializer) SetRootProvider(name string) { a.provider = name }

func (a *awsMaterializer) PrepareEnvironment(environment map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "auth.awsMaterializer.PrepareEnvironment")()

	addition, err := a.Environment()
	if err != nil {
		return nil, err
	}
	return awsCloud.PrepareEnvironment(environment, a.name, addition["AWS_SHARED_CREDENTIALS_FILE"], addition["AWS_CONFIG_FILE"], a.region), nil
}
