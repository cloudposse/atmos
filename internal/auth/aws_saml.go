package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/versent/saml2aws/v2"
	_ "github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"
)

type awsSaml struct {
	schema.IdentityDefaultConfig `yaml:",inline"`
	RoleArn                      string `yaml:"role_arn,omitempty" json:"role_arn,omitempty" mapstructure:"role_arn,omitempty"`
	SessionDuration              int32  `yaml:"session_duration,omitempty" json:"session_duration,omitempty" mapstructure:"session_duration,omitempty"`
}

// Input options for login.
type LoginOpts struct {
	// The IDP SAML login URL (e.g., Okta, Ping, ADFS, etc.)
	URL string
	// Optional: force a specific role ARN to assume. If empty, the first role will be used.
	RoleARN string
	// Optional AWS region for STS calls.
	Region string
	// Session duration in seconds (1h default if 0).
	SessionDuration int32
}

// Output credentials + metadata.
type LoginResult struct {
	Credentials  aws.Credentials
	AssumedRole  string
	PrincipalARN string
	Expires      time.Time
	AllRoles     []string
}

// Login opens a non-headless browser to authenticate with the IDP via saml2aws Browser provider,
// exchanges the SAML assertion for AWS creds, and returns them.
func Saml2AwsLogin(ctx context.Context, in LoginOpts) (*LoginResult, error) {
	_ = ensureSaml2awsStorageDir()
	if in.URL == "" {
		return nil, errors.New("URL is required")
	}
	if in.SessionDuration == 0 {
		in.SessionDuration = 3600
	}
	region := in.Region
	if region == "" {
		region = "us-east-1"
	}

	// Build the saml2aws IDP account using the Browser provider.
	idp := cfg.NewIDPAccount()
	idp.URL = in.URL
	idp.Provider = "Browser" // non-headless, real browser window
	idp.Headless = false
	idp.DownloadBrowser = true
	idp.SessionDuration = int(in.SessionDuration)

	if err := idp.Validate(); err != nil {
		return nil, fmt.Errorf("invalid idp account: %w", err)
	}

	// Create SAML client
	client, err := saml2aws.NewSAMLClient(idp)
	if err != nil {
		return nil, fmt.Errorf("create SAML client: %w", err)
	}

	// No username/password: the Browser provider will launch a UI and the user signs in.
	loginDetails := &creds.LoginDetails{
		URL:             idp.URL,
		DownloadBrowser: true,
	}

	log.Info("Launching IDP login in browser...", "url", in.URL)

	// Authenticate and get base64-encoded SAML assertion.
	assertionB64, err := client.Authenticate(loginDetails)
	if err != nil {
		return nil, fmt.Errorf("authenticate to IDP: %w", err)
	}
	log.Info("Received SAML assertion from IDP")

	decodedXML, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		return nil, fmt.Errorf("decode SAML assertion: %w", err)
	}

	rolesStr, err := saml2aws.ExtractAwsRoles(decodedXML)
	if err != nil {
		return nil, fmt.Errorf("extract AWS roles: %w", err)
	}
	roles, err := saml2aws.ParseAWSRoles(rolesStr)
	if err != nil {
		return nil, fmt.Errorf("parse AWS roles: %w", err)
	}
	if len(roles) == 0 {
		// Add a helpful hint for Google/Okta etc.
		return nil, fmt.Errorf("no AWS roles found in SAML assertion (IDP = Google?). Check that the IdP app is configured to include AWS role attributes (https://aws.amazon.com/SAML/Attributes) and that you’re assigned to at least one role")
	}

	// Pick target role
	targetRole := roles[0]
	if in.RoleARN != "" {
		selected, err := saml2aws.LocateRole(roles, in.RoleARN)
		if err != nil {
			return nil, fmt.Errorf("role %s not present in assertion: %w", in.RoleARN, err)
		}
		targetRole = selected
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	stsClient := sts.NewFromConfig(awsCfg)

	out, err := stsClient.AssumeRoleWithSAML(ctx, &sts.AssumeRoleWithSAMLInput{
		PrincipalArn:  aws.String(targetRole.PrincipalARN),
		RoleArn:       aws.String(targetRole.RoleARN),
		SAMLAssertion: aws.String(assertionB64),
		DurationSeconds: aws.Int32(func() int32 {
			// Respect requested duration within STS/account limits.
			if in.SessionDuration > 0 {
				return in.SessionDuration
			}
			return 3600
		}()),
	})
	if err != nil {
		return nil, fmt.Errorf("assume role with SAML: %w", err)
	}

	credsOut := out.Credentials
	if credsOut == nil {
		return nil, errors.New("no credentials returned by STS")
	}

	all := make([]string, 0, len(roles))
	for _, r := range roles {
		all = append(all, fmt.Sprintf("%s|%s", r.RoleARN, r.PrincipalARN))
	}

	log.Info("Assumed role with SAML",
		"role", targetRole.RoleARN,
		"principal", targetRole.PrincipalARN,
		"expires", credsOut.Expiration)

	return &LoginResult{
		Credentials: aws.Credentials{
			AccessKeyID:     aws.ToString(credsOut.AccessKeyId),
			SecretAccessKey: aws.ToString(credsOut.SecretAccessKey),
			SessionToken:    aws.ToString(credsOut.SessionToken),
			Source:          "saml2aws",
			CanExpire:       true,
			Expires:         aws.ToTime(credsOut.Expiration),
		},
		AssumedRole:  targetRole.RoleARN,
		PrincipalARN: targetRole.PrincipalARN,
		Expires:      aws.ToTime(credsOut.Expiration),
		AllRoles:     all,
	}, nil
}

func (i *awsSaml) Login() error {
	ctx := context.Background()
	// TODO check for existing credentials and expiry
	res, err := Saml2AwsLogin(ctx, LoginOpts{
		URL:             i.Url,
		RoleARN:         i.RoleArn,
		Region:          i.Region,
		SessionDuration: 3600,
	})
	log.Info("Success",
		"arn", res.AssumedRole,
		"expires", res.Expires,
	)

	WriteAwsCredentials(i.Profile, res.Credentials.AccessKeyID, res.Credentials.SecretAccessKey, res.Credentials.SessionToken)

	if err != nil {
		return err
	}
	return nil
}

func (i *awsSaml) Logout() error {
	return nil
}

func (i *awsSaml) Validate() error {
	return nil
}

// ensure saml2aws browser storage dir exists so storageState.json can be saved
func ensureSaml2awsStorageDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".aws", "saml2aws")
	return os.MkdirAll(dir, 0o700)
}
