package saml2aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRoles(t *testing.T) {
	roles := []string{
		"arn:aws:iam::456456456456:saml-provider/example-idp,arn:aws:iam::456456456456:role/admin",
		"arn:aws:iam::456456456456:role/admin,arn:aws:iam::456456456456:saml-provider/example-idp",
	}

	awsRoles, err := ParseAWSRoles(roles)

	assert.Nil(t, err)
	assert.Len(t, awsRoles, 2)

	for _, awsRole := range awsRoles {
		assert.Equal(t, "arn:aws:iam::456456456456:saml-provider/example-idp", awsRole.PrincipalARN)
		assert.Equal(t, "arn:aws:iam::456456456456:role/admin", awsRole.RoleARN)
	}

	roles = []string{""}
	awsRoles, err = ParseAWSRoles(roles)

	assert.NotNil(t, err)
	assert.Nil(t, awsRoles)
}

func TestParseAWSRolesErrors(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr string
	}{
		{
			name:    "single token",
			role:    "arn:aws:iam::123456789012:role/admin",
			wantErr: "invalid role string only 1 tokens",
		},
		{
			name:    "three tokens",
			role:    "arn:aws:iam::123456789012:role/a,arn:aws:iam::123456789012:saml-provider/b,arn:aws:iam::123456789012:role/c",
			wantErr: "invalid role string only 3 tokens",
		},
		{
			name:    "missing principal",
			role:    "arn:aws:iam::123456789012:role/admin,arn:aws:iam::123456789012:role/other",
			wantErr: "unable to locate PrincipalARN",
		},
		{
			name:    "missing role",
			role:    "arn:aws:iam::123456789012:saml-provider/a,arn:aws:iam::123456789012:saml-provider/b",
			wantErr: "unable to locate RoleARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles, err := ParseAWSRoles([]string{tt.role})
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
			assert.Nil(t, roles)
		})
	}
}

func TestParseAWSRolesGovCloudAndWhitespace(t *testing.T) {
	roles, err := ParseAWSRoles([]string{
		" arn:aws-us-gov:iam::123456789012:role/path/admin , arn:aws-us-gov:iam::123456789012:saml-provider/idp ",
	})
	if assert.NoError(t, err) && assert.Len(t, roles, 1) {
		assert.Equal(t, "arn:aws-us-gov:iam::123456789012:role/path/admin", roles[0].RoleARN)
		assert.Equal(t, "arn:aws-us-gov:iam::123456789012:saml-provider/idp", roles[0].PrincipalARN)
	}
}

// Regression: IAM permits commas in role names, so a comma only separates ARNs when another ARN follows.
func TestParseAWSRolesCommaInRoleName(t *testing.T) {
	for _, role := range []string{
		"arn:aws:iam::123456789012:role/admin,ops,arn:aws:iam::123456789012:saml-provider/idp",
		"arn:aws:iam::123456789012:saml-provider/idp,arn:aws:iam::123456789012:role/admin,ops",
	} {
		roles, err := ParseAWSRoles([]string{role})
		if assert.NoError(t, err) && assert.Len(t, roles, 1) {
			assert.Equal(t, "arn:aws:iam::123456789012:role/admin,ops", roles[0].RoleARN)
			assert.Equal(t, "arn:aws:iam::123456789012:saml-provider/idp", roles[0].PrincipalARN)
		}
	}
}

// Regression: IAM role paths may contain ",arn:" so a comma only separates ARNs when a complete ARN follows it.
func TestParseAWSRolesCommaARNInRolePath(t *testing.T) {
	const (
		roleARN      = "arn:aws:iam::123456789012:role/team,arn:blue/admin"
		principalARN = "arn:aws:iam::123456789012:saml-provider/idp"
	)
	for _, role := range []string{
		roleARN + "," + principalARN,
		principalARN + "," + roleARN,
	} {
		roles, err := ParseAWSRoles([]string{role})
		if assert.NoError(t, err) && assert.Len(t, roles, 1) {
			assert.Equal(t, roleARN, roles[0].RoleARN)
			assert.Equal(t, principalARN, roles[0].PrincipalARN)
		}
	}
}

// TestParseAWSRolesClassifiesByResourceType verifies a role path that mentions "saml-provider/" is not mistaken for a SAML provider.
func TestParseAWSRolesClassifiesByResourceType(t *testing.T) {
	const (
		provider = "arn:aws:iam::123456789012:saml-provider/idp"
		role     = "arn:aws:iam::123456789012:role/team:saml-provider/admin"
	)

	for _, value := range []string{provider + "," + role, role + "," + provider} {
		roles, err := ParseAWSRoles([]string{value})
		if assert.NoError(t, err) && assert.Len(t, roles, 1) {
			assert.Equal(t, provider, roles[0].PrincipalARN)
			assert.Equal(t, role, roles[0].RoleARN)
		}
	}

	// A role ARN whose path mentions saml-provider/ twice must not provide a principal.
	_, err := ParseAWSRoles([]string{role + ",arn:aws:iam::123456789012:role/other:saml-provider/x"})
	assert.Error(t, err)
}
