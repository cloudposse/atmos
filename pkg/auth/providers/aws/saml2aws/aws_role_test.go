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
			wantErr: "Invalid role string only 1 tokens",
		},
		{
			name:    "three tokens",
			role:    "arn:aws:iam::1:role/a,arn:aws:iam::1:saml-provider/b,arn:aws:iam::1:role/c",
			wantErr: "Invalid role string only 3 tokens",
		},
		{
			name:    "missing principal",
			role:    "arn:aws:iam::123456789012:role/admin,arn:aws:iam::123456789012:role/other",
			wantErr: "Unable to locate PrincipalARN",
		},
		{
			name:    "missing role",
			role:    "arn:aws:iam::123456789012:saml-provider/a,arn:aws:iam::123456789012:saml-provider/b",
			wantErr: "Unable to locate RoleARN",
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
