package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/telemetry"

	// tuiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

// awsUserSecret is what we persist in the keyring
type awsUserSecret struct {
	// Stored by `atmos auth user configure` and represents the long lived credentials
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	MfaArn          string `json:"mfa_arn,omitempty"`

	// ShortTerm that can be cached from run to run
	LastUpdated time.Time                  `json:"last_updated"`
	ExpiresAt   time.Time                  `json:"expires_at"`
	TokenOutput *sts.GetSessionTokenOutput `json:"token_output"`
}

type awsUser struct {
	Common          schema.ProviderDefaultConfig `yaml:",inline"`
	schema.Identity `yaml:",inline"`
}

// newKeyringStore is a package-level factory to create a keyring-backed GenericStore.
// Tests can override this to inject a mock store.
var newKeyringStore = func() authstore.GenericStore { return authstore.NewKeyringAuthStore() }

func NewAwsUserFactory(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
	data := &awsUser{
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

func (i *awsUser) Validate() error {
	if i.Common.Profile == "" {
		return fmt.Errorf("profile is required for aws/user")
	}
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1"
	}
	return nil
}

// Login retrieves long-lived user credentials from keyring and exchanges them for short-lived session
// credentials via STS GetSessionToken. If an MFA device ARN is configured, the user is prompted for an MFA code.
func (i *awsUser) Login() error {
	store := newKeyringStore()
	key := i.keyringAlias()
	ctx := context.Background()
	var secret awsUserSecret
	if err := store.GetInto(key, &secret); err != nil {
		return fmt.Errorf("no stored credentials for alias %q: %w", key, err)
	}
	if secret.AccessKeyID == "" || secret.SecretAccessKey == "" {
		return fmt.Errorf("stored credentials for %q are incomplete, pleae run `atmos auth user configure` to configure the user", key)
	}
	if time.Until(secret.ExpiresAt) > 30*time.Minute {
		// Valid token, proceed
		log.Debug("Cached token found", "key", key)
	} else {
		// Base cfg using your long-lived keys
		baseProv := credentials.NewStaticCredentialsProvider(secret.AccessKeyID, secret.SecretAccessKey, "")
		baseCfg, err := config.LoadDefaultConfig(
			ctx,
			config.WithRegion(i.Common.Region),
			config.WithCredentialsProvider(baseProv),
		)
		if err != nil {
			return fmt.Errorf("load base AWS config: %w", err)
		}

		// Call STS:GetSessionToken (+MFA when mfaSerial is provided)
		stsClient := sts.NewFromConfig(baseCfg)

		input := &sts.GetSessionTokenInput{
			DurationSeconds: aws.Int32(int32(12 * time.Hour.Seconds())),
		}

		// If we have an ARN for MFA and it's not CI, prompt for the MFA token.
		if secret.MfaArn != "" && !telemetry.IsCI() {
			var code string
			if err := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().Title("Enter MFA code").Value(&code),
				),
			).Run(); err != nil {
				return fmt.Errorf("read MFA code: %w", err)
			}
			if code == "" {
				return fmt.Errorf("MFA code is required for device %s", secret.MfaArn)
			}
			input.SerialNumber = aws.String(secret.MfaArn)
			input.TokenCode = aws.String(code)
		}

		// Get the actual token
		out, err := stsClient.GetSessionToken(ctx, input)
		if err != nil {
			return fmt.Errorf("GetSessionToken: %w", err)
		}
		if out.Credentials == nil {
			return fmt.Errorf("GetSessionToken returned no credentials")
		}
		// Populate information to be stored.
		secret.TokenOutput = out
		secret.ExpiresAt = *out.Credentials.Expiration
		secret.LastUpdated = time.Now()
		err = store.SetAny(key, secret)
		if err != nil {
			return err
		}
	}

	// Build a new Credentials Provider using either the cached or refreshed credentials
	tempProv := credentials.NewStaticCredentialsProvider(
		aws.ToString(secret.TokenOutput.Credentials.AccessKeyId),
		aws.ToString(secret.TokenOutput.Credentials.SecretAccessKey),
		aws.ToString(secret.TokenOutput.Credentials.SessionToken),
	)
	log.Info("✅ Obtained MFA-backed session credentials",
		"expires", secret.TokenOutput.Credentials.Expiration.Local().Format(time.RFC1123))

	if err := WriteAwsCredentials(
		i.Common.Profile,
		tempProv.Value.AccessKeyID,
		tempProv.Value.SecretAccessKey,
		tempProv.Value.SessionToken,
		i.Provider,
	); err != nil {
		return err
	}
	log.Info("✅ Generated short-lived AWS session credentials", "profile", i.Common.Profile, "expires", secret.TokenOutput.Credentials.Expiration.Local().Format(time.RFC1123))
	return nil
}

// AssumeRole is a no-op for static user credentials
func (i *awsUser) AssumeRole() error { return nil }

func (i *awsUser) SetEnvVars(info *schema.ConfigAndStacksInfo) error {
	log.Info("Setting AWS environment variables")

	err := SetAwsEnvVars(info, i.Identity.Identity, i.Provider, i.Common.Region)
	if err != nil {
		return err
	}

	// Merge identity-specific env overrides (preserve key casing)
	MergeIdentityEnvOverrides(info, i.Env)

	err = UpdateAwsAtmosConfig(i.Provider, i.Identity.Identity, i.Common.Profile, i.Common.Region, i.RoleArn)
	if err != nil {
		return err
	}

	return nil
}

func (i *awsUser) Logout() error {
	return RemoveAwsCredentials(i.Common.Profile)
}

func (i *awsUser) keyringAlias() string {
	// Ensure uniqueness across identities sharing a provider
	if i.Identity.Identity != "" {
		return fmt.Sprintf("%s/%s", i.Provider, i.Identity.Identity)
	}
	return i.Provider
}
