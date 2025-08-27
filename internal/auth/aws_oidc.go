package auth

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

// awsOidc implements LoginMethod for assuming an AWS role with an OIDC web identity token
// using STS AssumeRoleWithWebIdentity.
type awsOidc struct {
	Common          schema.ProviderDefaultConfig `yaml:",inline"`
	schema.Identity `yaml:",inline"`

	// Optional settings
	SessionDuration      int32  `yaml:"session_duration,omitempty" json:"session_duration,omitempty" mapstructure:"session_duration,omitempty"`
	WebIdentityTokenFile string `yaml:"web_identity_token_file,omitempty" json:"web_identity_token_file,omitempty" mapstructure:"web_identity_token_file,omitempty"`
	WebIdentityToken     string `yaml:"web_identity_token,omitempty" json:"web_identity_token,omitempty" mapstructure:"web_identity_token,omitempty"`
	RoleSessionName      string `yaml:"role_session_name,omitempty" json:"role_session_name,omitempty" mapstructure:"role_session_name,omitempty"`

	// Internal/testing: allow injecting a custom STS client (not marshaled)
	stsClient stsAPI `yaml:"-" json:"-"`
}

// stsAPI is the minimal interface we use from the STS client (for testing/DI)
type stsAPI interface {
    AssumeRoleWithWebIdentity(ctx context.Context, params *sts.AssumeRoleWithWebIdentityInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error)
}

func NewAwsOidcFactory(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
	data := &awsOidc{
		Identity: NewIdentity(),
	}
	b, err := yaml.Marshal(config.Providers[provider])
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, data)
	setDefaults(&data.Common, provider, config)
	data.Identity.Identity = identity
	return data, err
}

// Validate checks minimum required fields
func (i *awsOidc) Validate() error {
	if i.RoleArn == "" {
		return fmt.Errorf("role_arn is required for aws/oidc")
	}
	// default region
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1"
	}
	// Session duration default
	if i.SessionDuration == 0 {
		i.SessionDuration = 3600
	}
	// Ensure we can obtain a token one way or another
	if i.WebIdentityToken == "" && i.WebIdentityTokenFile == "" && os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
		log.Warn("No web identity token or token file configured; will still attempt during Login using env")
	}
	return nil
}

// Login resolves token source but does not contact AWS.
func (i *awsOidc) Login() error {
	// Nothing to authenticate with an IdP here; we rely on provided token or file.
	// However, provide a friendly log and basic check.
	if i.WebIdentityToken == "" {
		if i.WebIdentityTokenFile == "" {
			i.WebIdentityTokenFile = os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
		}
		if i.WebIdentityTokenFile != "" {
			b, err := ioutil.ReadFile(i.WebIdentityTokenFile)
			if err != nil {
				return fmt.Errorf("failed reading web identity token file %s: %w", i.WebIdentityTokenFile, err)
			}
			i.WebIdentityToken = string(b)
		}
	}
	if i.WebIdentityToken == "" {
		return fmt.Errorf("no web identity token available; set web_identity_token, web_identity_token_file, or AWS_WEB_IDENTITY_TOKEN_FILE")
	}
	log.Info("✅ OIDC token loaded for AssumeRoleWithWebIdentity")
	return nil
}

// AssumeRole exchanges the OIDC token for AWS credentials using STS.
func (i *awsOidc) AssumeRole() error {
	if i.WebIdentityToken == "" {
		return fmt.Errorf("no web identity token available, please login first")
	}

	ctx := context.Background()

	// Create STS client with anonymous credentials (AssumeRoleWithWebIdentity is unsigned)
	var stsClient stsAPI
	if i.stsClient != nil {
		stsClient = i.stsClient
	} else {
		opts := sts.Options{
			Region:      i.Common.Region,
			Credentials: aws.AnonymousCredentials{},
		}
		        if ep := os.Getenv("AWS_STS_ENDPOINT_URL"); ep != "" {
            resolver := sts.EndpointResolverFunc(func(region string, _ sts.EndpointResolverOptions) (aws.Endpoint, error) {
                return aws.Endpoint{URL: ep, HostnameImmutable: true}, nil
            })
            opts.EndpointResolver = resolver
        }
		stsClient = sts.New(opts)
	}

	sessionName := i.RoleSessionName
	if sessionName == "" {
		sessionName = fmt.Sprintf("AtmosOIDC-%d", time.Now().Unix())
	}

	out, err := stsClient.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(i.RoleArn),
		RoleSessionName:  aws.String(sessionName),
		WebIdentityToken: aws.String(i.WebIdentityToken),
		DurationSeconds:  aws.Int32(i.SessionDuration),
	})
	if err != nil {
		return fmt.Errorf("failed to assume role with web identity: %w", err)
	}

	if out.Credentials == nil {
		return fmt.Errorf("no credentials returned from AssumeRoleWithWebIdentity")
	}

	WriteAwsCredentials(
		i.Common.Profile,
		aws.ToString(out.Credentials.AccessKeyId),
		aws.ToString(out.Credentials.SecretAccessKey),
		aws.ToString(out.Credentials.SessionToken),
		"aws/oidc",
	)

	log.Info("✅ Successfully assumed role via OIDC",
		"role", i.RoleArn,
		"profile", i.Common.Profile,
		"expires", out.Credentials.Expiration.Local().Format(time.RFC1123),
	)
	return nil
}

func (i *awsOidc) SetEnvVars(info *schema.ConfigAndStacksInfo) error {
	log.Info("Setting AWS environment variables")
	if err := SetAwsEnvVars(info, i.Identity.Identity, i.Provider, i.Common.Region); err != nil {
		return err
	}
	MergeIdentityEnvOverrides(info, i.Env)
	if err := UpdateAwsAtmosConfig(i.Provider, i.Identity.Identity, i.Common.Profile, i.Common.Region, i.RoleArn); err != nil {
		return err
	}
	return nil
}

func (i *awsOidc) Logout() error {
	return RemoveAwsCredentials(i.Common.Profile)
}
