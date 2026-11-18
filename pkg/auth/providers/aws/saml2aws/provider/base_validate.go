package provider

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

// Static validation errors. Atmos fork: the messages are unchanged from upstream, but callers can now match them with errors.Is.
var (
	ErrEmptyURL      = errors.New("empty URL")
	ErrEmptyUsername = errors.New("empty username")
	ErrEmptyPassword = errors.New("empty password")
)

type ValidateBase struct{}

func (ac *ValidateBase) Validate(ld *creds.LoginDetails) error {
	if ld.URL == "" {
		return ErrEmptyURL
	}
	if ld.Username == "" {
		return ErrEmptyUsername
	}
	if ld.Password == "" {
		return ErrEmptyPassword
	}
	return nil
}
