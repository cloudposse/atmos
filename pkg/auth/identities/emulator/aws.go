package emulator

import (
	"context"
	"fmt"

	awscloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWS SDK environment variables carried in the resolved emulator profile.
const (
	envAWSEndpointURL   = "AWS_ENDPOINT_URL"
	envAWSAccessKeyID   = "AWS_ACCESS_KEY_ID"
	envAWSSecretKey     = "AWS_SECRET_ACCESS_KEY"
	envAWSRegion        = "AWS_REGION"
	envAWSDefaultRegion = "AWS_DEFAULT_REGION"
)

// setAWSAuthContext populates params.AuthContext.AWS for an aws/emulator identity.
//
// In-process AWS SDK clients — stores (`!store`, store hooks), the secrets engine,
// and anything else that talks to AWS from inside the atmos process — build their
// config from AuthContext.AWS (a shared-credentials file + profile + region +
// endpoint), NOT from the subprocess environment variables that PrepareEnvironment
// injects for Terraform. We therefore write a credentials file with the emulator's
// static dummy credentials and point the auth context at the live emulator
// endpoint — the same shape SetAuthContext produces for real AWS identities — so
// those in-process consumers reach the emulator just like Terraform does.
func (i *Identity) setAWSAuthContext(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "emulator.Identity.setAWSAuthContext")()

	// The authoritative stack for this operation is the manager's stackInfo; fall
	// back to the stack injected at construction. Without a stack we cannot scope
	// the emulator instance — skip (leaving AuthContext.AWS nil) rather than fail,
	// so auth flows that don't actually need the in-process AWS context still work.
	stack := i.stack
	if params.StackInfo != nil && params.StackInfo.Stack != "" {
		stack = params.StackInfo.Stack
	}
	if stack == "" {
		return nil
	}

	// Surface a resolver failure rather than swallowing it: an unresolvable emulator
	// (e.g. not running) must produce a clear error here instead of leaving the
	// in-process AWS context unpopulated and letting consumers fail later against real
	// AWS with a confusing credentials error.
	env, _, err := i.resolver.ResolveEmulator(ctx, stack, i.config.Emulator)
	if err != nil {
		return fmt.Errorf("resolve emulator %q for identity %q: %w", i.config.Emulator, i.Name(), err)
	}

	region := env[envAWSRegion]
	if region == "" {
		region = env[envAWSDefaultRegion]
	}

	creds := &types.AWSCredentials{
		AccessKeyID:     env[envAWSAccessKeyID],
		SecretAccessKey: env[envAWSSecretKey],
		Region:          region,
	}

	// Write the shared credentials + config files under the root provider name,
	// keyed by this identity's name as the profile. An empty base path resolves to
	// the default XDG location; the realm provides multi-repository isolation.
	fm, err := awscloud.NewAWSFileManager("", i.realm)
	if err != nil {
		return fmt.Errorf("emulator identity %q: AWS file manager: %w", i.Name(), err)
	}
	if err := fm.WriteCredentials(params.ProviderName, params.IdentityName, creds); err != nil {
		return fmt.Errorf("emulator identity %q: write AWS credentials: %w", i.Name(), err)
	}
	if err := fm.WriteConfig(params.ProviderName, params.IdentityName, region, ""); err != nil {
		return fmt.Errorf("emulator identity %q: write AWS config: %w", i.Name(), err)
	}

	params.AuthContext.AWS = &schema.AWSAuthContext{
		CredentialsFile: fm.GetCredentialsPath(params.ProviderName),
		ConfigFile:      fm.GetConfigPath(params.ProviderName),
		Profile:         params.IdentityName,
		Region:          region,
		EndpointURL:     env[envAWSEndpointURL],
	}
	return nil
}
