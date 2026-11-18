package cfg

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
)

// Atmos fork: the ini-file ConfigManager (SaveIDPAccount/LoadIDPAccount, ~/.saml2aws) was removed.
// Atmos builds IDPAccount values from atmos.yaml and never reads or writes the saml2aws config file.

const (
	// DefaultAmazonWebservicesURN URN used when authenticating to aws using SAML
	// NOTE: This only needs to be changed to log into GovCloud
	DefaultAmazonWebservicesURN = "urn:amazon:webservices"

	// DefaultSessionDuration this is the default session duration which can be overridden in the AWS console
	// see https://aws.amazon.com/blogs/security/enable-federated-api-access-to-your-aws-resources-for-up-to-12-hours-using-iam-roles/
	DefaultSessionDuration = 3600

	// DefaultProfile this is the default profile name used to save the credentials in the aws cli
	DefaultProfile = "saml"
)

// IDPAccount saml IDP account
//
// Atmos fork: upstream's DownloadBrowser field was removed. Nothing in the library read it
// (only the saml2aws CLI did, through SAML2AWS_AUTO_BROWSER_DOWNLOAD); the browser driver
// installs browsers when creds.LoginDetails.DownloadBrowser is set.
type IDPAccount struct {
	Name                  string
	URL                   string
	Username              string
	Provider              string
	BrowserType           string // used by 'Browser' Provider
	BrowserExecutablePath string // used by 'Browser' Provider
	BrowserAutoFill       bool   // used by 'Browser' Provider
	MFA                   string
	SkipVerify            bool
	Timeout               int
	AmazonWebservicesURN  string
	SessionDuration       int
	Profile               string
	RoleARN               string
	PolicyFile            string
	PolicyARNs            string
	Region                string
	HttpAttemptsCount     string
	HttpRetryDelay        string
	CredentialsFile       string
	SAMLCache             bool
	SAMLCacheFile         string
	TargetURL             string
	DisableRememberDevice bool   // used by Okta
	DisableSessions       bool   // used by Okta
	BrowserDriverDir      string // used by browser; hide from user if not set
	Headless              bool   // used by browser
	Prompter              string
}

func (ia IDPAccount) String() string {
	var oktaCfg string
	if ia.Provider == "Okta" {
		oktaCfg = fmt.Sprintf(`
  DisableSessions: %v
  DisableRememberDevice: %v`, ia.DisableSessions, ia.DisableRememberDevice)
	}

	return fmt.Sprintf(`account {%s
  URL: %s
  Username: %s
  Provider: %s
  MFA: %s
  SkipVerify: %v
  AmazonWebservicesURN: %s
  SessionDuration: %d
  Profile: %s
  RoleARN: %s
  Region: %s
}`, oktaCfg, ia.URL, ia.Username, ia.Provider, ia.MFA, ia.SkipVerify, ia.AmazonWebservicesURN, ia.SessionDuration, ia.Profile, ia.RoleARN, ia.Region)
}

// Validate validate the required / expected fields are set
func (ia *IDPAccount) Validate() error {
	if ia.URL == "" {
		return errors.New("URL empty in idp account")
	}

	_, err := url.Parse(ia.URL)
	if err != nil {
		return errors.New("URL parse failed")
	}

	if ia.Provider == "" {
		return errors.New("Provider empty in idp account")
	}

	if ia.Provider != "Browser" {
		if ia.MFA == "" {
			return errors.New("MFA empty in idp account")
		}
	}

	if ia.Profile == "" {
		return errors.New("Profile empty in idp account")
	}

	if err := prompter.ValidateAndSetPrompter(ia.Prompter); err != nil {
		return err
	}

	return nil
}

// NewIDPAccount Create an idp account and fill in any default fields with sane values
func NewIDPAccount() *IDPAccount {
	return &IDPAccount{
		AmazonWebservicesURN: DefaultAmazonWebservicesURN,
		SessionDuration:      DefaultSessionDuration,
		Profile:              DefaultProfile,
	}
}
