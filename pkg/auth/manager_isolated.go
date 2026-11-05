package auth

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AuthenticateIsolated authenticates identityName on a separate manager that shares
// this manager's effective configuration, credential store, validator, realm, and
// stack metadata, but owns its own provider/identity instances, chain, and auth
// context. The receiver's chain, stack info, and auth context are never modified.
func (m *manager) AuthenticateIsolated(ctx context.Context, identityName string) (*schema.AuthContext, error) {
	defer perf.Track(nil, "auth.Manager.AuthenticateIsolated")()

	isolatedStackInfo := &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{}}
	if m.stackInfo != nil {
		isolatedStackInfo.Stack = m.stackInfo.Stack
		isolatedStackInfo.ComponentFromArg = m.stackInfo.ComponentFromArg
		isolatedStackInfo.ComponentPrompted = m.stackInfo.ComponentPrompted
		isolatedStackInfo.StackPrompted = m.stackInfo.StackPrompted
		if m.stackInfo.ProfilesFromArg != nil {
			isolatedStackInfo.ProfilesFromArg = append([]string(nil), m.stackInfo.ProfilesFromArg...)
		}
	}

	isolated, err := NewAuthManager(CopyGlobalAuthConfig(m.config), m.credentialStore, m.validator, isolatedStackInfo, m.cliConfigPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}

	if _, err := isolated.Authenticate(ctx, identityName); err != nil {
		return nil, err
	}

	return isolatedStackInfo.AuthContext, nil
}
