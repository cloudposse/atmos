package auth

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

type awsSaml struct {
	schema.IdentityDefaultConfig

	Role   string `yaml:"role,omitempty" json:"role,omitempty" mapstructure:"role,omitempty"`
	IdpArn string `yaml:"idp_arn,omitempty" json:"idp_arn,omitempty" mapstructure:"idp_arn,omitempty"`
	Url    string `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url,omitempty"`
}

func (i *awsSaml) Login() error {
	// actual logic
	return nil
}

func (i *awsSaml) Logout() error {
	return nil
}

func (i *awsSaml) Validate() error {
	return nil
}
