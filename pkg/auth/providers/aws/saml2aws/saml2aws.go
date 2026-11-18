package saml2aws

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider/adfs"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider/browser"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider/googleapps"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider/okta"
)

// Atmos fork: only the SAML drivers Atmos documents (Browser, GoogleApps, Okta and ADFS) are kept.
// Upstream's other IdP providers (AzureAD, ADFS2, Ping*, JumpCloud, OneLogin, Authentik, KeyCloak,
// Shibboleth*, F5APM, Akamai, Shell, NetIQ and Auth0) were removed. The Browser driver works with
// any IdP that ends in the AWS SAML sign-in page.

var (
	// ErrUnsupportedProvider is returned when the configured SAML driver is not one Atmos supports.
	ErrUnsupportedProvider = errors.New("unsupported SAML driver")

	// ErrInvalidMFA is returned when the configured MFA type is not valid for the SAML driver.
	ErrInvalidMFA = errors.New("invalid MFA type")
)

// ProviderList list of providers with their MFAs
type ProviderList map[string][]string

// MFAsByProvider a list of providers with their respective supported MFAs
var MFAsByProvider = ProviderList{
	"ADFS":       []string{"Auto", "VIP", "Azure", "Defender"},
	"Okta":       []string{"Auto", "PUSH", "DUO", "SMS", "EMAIL", "TOTP", "OKTA", "FIDO", "YUBICO TOKEN:HARDWARE", "SYMANTEC"}, // automatically detects DUO, SMS, ToTP, and FIDO
	"GoogleApps": []string{"Auto"},                                                                                             // automatically detects ToTP
	"Browser":    []string{"Auto"},
}

// Names get a list of provider names
func (mfbp ProviderList) Names() []string {
	keys := []string{}
	for k := range mfbp {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// Mfas retrieve a sorted list of mfas from the provider list
func (mfbp ProviderList) Mfas(provider string) []string {
	mfas := append([]string(nil), mfbp[provider]...)

	sort.Strings(mfas)

	return mfas
}

func (mfbp ProviderList) stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func invalidMFA(provider string, mfa string) bool {
	supportedMfas := MFAsByProvider.Mfas(provider)
	return !MFAsByProvider.stringInSlice(mfa, supportedMfas)
}

// SAMLClient client interface
type SAMLClient interface {
	Authenticate(loginDetails *creds.LoginDetails) (string, error)
	Validate(loginDetails *creds.LoginDetails) error
}

// NewSAMLClient create a new SAML client
func NewSAMLClient(idpAccount *cfg.IDPAccount) (SAMLClient, error) {
	if idpAccount.Provider != "Browser" && invalidMFA(idpAccount.Provider, idpAccount.MFA) {
		if _, known := MFAsByProvider[idpAccount.Provider]; known {
			return nil, fmt.Errorf("%w: %v for %v provider", ErrInvalidMFA, idpAccount.MFA, idpAccount.Provider)
		}
	}

	switch idpAccount.Provider {
	case "ADFS":
		return adfs.New(idpAccount)
	case "Okta":
		return okta.New(idpAccount)
	case "GoogleApps":
		return googleapps.New(idpAccount)
	case "Browser":
		return browser.New(idpAccount)
	default:
		return nil, fmt.Errorf("%w %q: supported drivers are %s",
			ErrUnsupportedProvider, idpAccount.Provider, strings.Join(MFAsByProvider.Names(), ", "))
	}
}
