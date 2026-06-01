package auth

import (
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewDefaultManager constructs an AuthManager with the default credential store and validator.
// It is the package-level equivalent of cmd/auth.CreateAuthManager, usable by non-cmd packages
// (e.g., ambient credential brokers in pkg/auth/broker) that must build a manager without taking
// a dependency on the command layer.
func NewDefaultManager(authConfig *schema.AuthConfig, cliConfigPath string) (types.AuthManager, error) {
	defer perf.Track(nil, "auth.NewDefaultManager")()

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	return NewAuthManager(authConfig, credStore, validator, authStackInfo, cliConfigPath)
}
