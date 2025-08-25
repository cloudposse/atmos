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
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/versent/saml2aws/v2"
	_ "github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"
)

type awsSaml struct {
	Common          schema.ProviderDefaultConfig `yaml:",inline"`
	schema.Identity `yaml:",inline"`

	SessionDuration int32 `yaml:"session_duration,omitempty" json:"session_duration,omitempty" mapstructure:"session_duration,omitempty"`

	// Store SAML assertion and roles between Login and AssumeRole steps
	samlAssertion string
	samlRoles     []*saml2aws.AWSRole
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
	idp.BrowserAutoFill = true
	idp.BrowserType = "chrome" // maybe we try iterating over other browsers to see what launches?
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

	log.Debug("Launching IDP login in browser...", "url", in.URL)

	// Authenticate and get base64-encoded SAML assertion.
	assertionB64, err := client.Authenticate(loginDetails)
	if err != nil {
		return nil, fmt.Errorf("authenticate to IDP: %w", err)
	}
	log.Debug("Received SAML assertion from IDP")

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

	stsClient := sts.New(sts.Options{
		Region: region,
	})
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

	log.Debug("Assumed role with SAML",
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

func (i *awsSaml) Validate() error {
	if i.Common.Url == "" {
		return fmt.Errorf("url is required for AWS SAML")
	}

	if i.Common.Profile == "" {
		return fmt.Errorf("profile is required for AWS SAML")
	}

	return nil
}

// Login authenticates with the IdP and gets the SAML assertion
func (i *awsSaml) Login() error {
	if IsInDocker() {
		log.Info("Skipping saml login command in docker", "reason", "Unsupported")
		return nil
	}

	// Build the saml2aws IDP account using the Browser provider.
	idp := cfg.NewIDPAccount()
	idp.URL = i.Common.Url
	idp.Provider = "Browser" // non-headless, real browser window
	idp.Headless = false
	idp.BrowserAutoFill = true
	idp.BrowserType = "chrome"
	idp.SessionDuration = int(i.SessionDuration)

	if err := idp.Validate(); err != nil {
		return fmt.Errorf("invalid idp account: %w", err)
	}

	// Create SAML client
	client, err := saml2aws.NewSAMLClient(idp)
	if err != nil {
		return fmt.Errorf("create SAML client: %w", err)
	}

	// No username/password: the Browser provider will launch a UI and the user signs in.
	loginDetails := &creds.LoginDetails{
		URL:             idp.URL,
		DownloadBrowser: true,
	}

	log.Debug("Launching IDP login in browser...", "url", i.Common.Url)

	// Authenticate and get base64-encoded SAML assertion.
	assertionB64, err := client.Authenticate(loginDetails)
	if err != nil {
		return fmt.Errorf("authenticate to IDP: %w", err)
	}
	log.Debug("Received SAML assertion from IDP")

	// Store the assertion for the AssumeRole step
	i.samlAssertion = assertionB64

	decodedXML, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		return fmt.Errorf("decode SAML assertion: %w", err)
	}

	rolesStr, err := saml2aws.ExtractAwsRoles(decodedXML)
	if err != nil {
		return fmt.Errorf("extract AWS roles: %w", err)
	}
	roles, err := saml2aws.ParseAWSRoles(rolesStr)
	if err != nil {
		return fmt.Errorf("parse AWS roles: %w", err)
	}
	if len(roles) == 0 {
		// Add a helpful hint for Google/Okta etc.
		return fmt.Errorf("no AWS roles found in SAML assertion (IDP = Google?). Check that the IdP app is configured to include AWS role attributes (https://aws.amazon.com/SAML/Attributes) and that you're assigned to at least one role")
	}

	// Store roles for the AssumeRole step
	i.samlRoles = roles

	log.Info("✅ Successfully authenticated with IdP", "url", i.Common.Url)
	return nil
}

// AssumeRole uses the SAML assertion from Login to assume an AWS role
func (i *awsSaml) AssumeRole() error {
	if i.samlAssertion == "" {
		return fmt.Errorf("no SAML assertion available, please login first")
	}

	if len(i.samlRoles) == 0 {
		return fmt.Errorf("no roles available from SAML assertion")
	}

	ctx := context.Background()

	// Pick target role
	targetRole := i.samlRoles[0]
	if i.RoleArn != "" {
		selected, err := saml2aws.LocateRole(i.samlRoles, i.RoleArn)
		if err != nil {
			return fmt.Errorf("role %s not present in assertion: %w", i.RoleArn, err)
		}
		targetRole = selected
	}

	region := i.Common.Region
	if region == "" {
		region = "us-east-1"
	}

	// Set session duration if not specified
	sessionDuration := i.SessionDuration
	if sessionDuration == 0 {
		sessionDuration = 3600
	}

	stsClient := sts.New(sts.Options{
		Region: region,
	})
	out, err := stsClient.AssumeRoleWithSAML(ctx, &sts.AssumeRoleWithSAMLInput{
		PrincipalArn:    aws.String(targetRole.PrincipalARN),
		RoleArn:         aws.String(targetRole.RoleARN),
		SAMLAssertion:   aws.String(i.samlAssertion),
		DurationSeconds: aws.Int32(sessionDuration),
	})
	if err != nil {
		return fmt.Errorf("assume role with SAML: %w", err)
	}

	credsOut := out.Credentials
	if credsOut == nil {
		return errors.New("no credentials returned by STS")
	}

	WriteAwsCredentials(
		i.Common.Profile,
		aws.ToString(credsOut.AccessKeyId),
		aws.ToString(credsOut.SecretAccessKey),
		aws.ToString(credsOut.SessionToken),
		i.Provider,
	)

	log.Info("✅ Assumed role with SAML",
		"role", targetRole.RoleARN,
		"profile", i.Common.Profile,
		"expires", aws.ToTime(credsOut.Expiration))

	return nil
}

func (i *awsSaml) SetEnvVars(info *schema.ConfigAndStacksInfo) error {
	return SetAwsEnvVars(info, i.Common.Profile, i.Provider, i.Common.Region)
}

func (i *awsSaml) Logout() error {
	return RemoveAwsCredentials(i.Common.Profile)
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
