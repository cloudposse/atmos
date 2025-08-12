package auth

import (
	"github.com/cloudposse/atmos/pkg/schema"
	_ "github.com/versent/saml2aws/v2"
)

type awsAssumeRole struct {
	Common   schema.IdentityProviderDefaultConfig `yaml:",inline"`
	Identity schema.Identity                      `yaml:",inline"`

	RoleArn string `yaml:"role_arn,omitempty" json:"role_arn,omitempty" mapstructure:"role_arn,omitempty"`
}

func (i *awsAssumeRole) Login() error {
	return nil
}

func (i *awsAssumeRole) Logout() error {
	return nil
}

func (i *awsAssumeRole) Validate() error {
	return nil
}
