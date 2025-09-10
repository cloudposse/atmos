package aws

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/schema"
)

func TestSSOProvider_getSessionDuration(t *testing.T) {
    // Default when no session configured
    p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
    require.NoError(t, err)
    assert.Equal(t, 60, p.getSessionDuration())

    // Valid duration string
    p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "15m"}})
    require.NoError(t, err)
    assert.Equal(t, 15, p.getSessionDuration())

    // Invalid duration string -> default
    p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "bogus"}})
    require.NoError(t, err)
    assert.Equal(t, 60, p.getSessionDuration())
}

func TestSSOProvider_Validate_Errors(t *testing.T) {
    // Create valid provider, then mutate fields to trigger Validate errors
    p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
    require.NoError(t, err)

    p.region = ""
    assert.Error(t, p.Validate())

    p.region = "us-east-1"
    p.startURL = ""
    assert.Error(t, p.Validate())
}
