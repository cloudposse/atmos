package cfg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Atmos fork: the upstream ConfigManager tests were removed with the ini-file ConfigManager.

func TestIDPAccountString(t *testing.T) {
	idpAccount := NewIDPAccount()
	idpAccount.URL = "https://id.whatever.com/#/hash"
	idpAccount.Provider = "Okta"
	idpAccount.DisableRememberDevice = true

	s := idpAccount.String()
	require.Contains(t, s, "urn:amazon:webservices\n")
	require.Contains(t, s, "URL: https://id.whatever.com/#/hash\n")
	require.Contains(t, s, "DisableSessions: false\n")
	require.Contains(t, s, "DisableRememberDevice: true\n")
}

func TestIDPAccountStringNonOkta(t *testing.T) {
	idpAccount := NewIDPAccount()
	idpAccount.Provider = "Browser"

	require.NotContains(t, idpAccount.String(), "DisableSessions")
}

func TestNewIDPAccountDefaults(t *testing.T) {
	idpAccount := NewIDPAccount()

	require.Equal(t, DefaultAmazonWebservicesURN, idpAccount.AmazonWebservicesURN)
	require.Equal(t, DefaultSessionDuration, idpAccount.SessionDuration)
	require.Equal(t, DefaultProfile, idpAccount.Profile)
}

func TestIDPAccountValidate(t *testing.T) {
	valid := func() *IDPAccount {
		a := NewIDPAccount()
		a.URL = "https://id.example.com"
		a.Provider = "Okta"
		a.MFA = "Auto"
		return a
	}

	tests := []struct {
		name    string
		mutate  func(a *IDPAccount)
		wantErr string
	}{
		{name: "valid", mutate: func(*IDPAccount) {}},
		{name: "browser does not need MFA", mutate: func(a *IDPAccount) { a.Provider = "Browser"; a.MFA = "" }},
		{name: "missing URL", mutate: func(a *IDPAccount) { a.URL = "" }, wantErr: "URL empty"},
		{name: "unparseable URL", mutate: func(a *IDPAccount) { a.URL = "://bad" }, wantErr: "URL parse failed"},
		{name: "missing provider", mutate: func(a *IDPAccount) { a.Provider = "" }, wantErr: "provider empty"},
		{name: "missing MFA", mutate: func(a *IDPAccount) { a.MFA = "" }, wantErr: "MFA empty"},
		{name: "missing profile", mutate: func(a *IDPAccount) { a.Profile = "" }, wantErr: "profile empty"},
		{name: "invalid prompter", mutate: func(a *IDPAccount) { a.Prompter = "nope" }, wantErr: "is not valid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := valid()
			tt.mutate(a)
			err := a.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
