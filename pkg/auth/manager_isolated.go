package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

// AuthenticateIsolated authenticates identityName on a separate manager that shares
// this manager's effective configuration, credential store, validator, realm, and
// stack metadata, but owns its own provider/identity instances, chain, and auth
// context. The receiver's chain, stack info, and auth context are never modified.
func (m *manager) AuthenticateIsolated(ctx context.Context, identityName string) (*schema.AuthContext, error) {
	defer perf.Track(nil, "auth.Manager.AuthenticateIsolated")()
	isolated, stackInfo, err := m.newIsolatedManager()
	if err != nil {
		return nil, err
	}
	if _, err := isolated.Authenticate(ctx, identityName); err != nil {
		return nil, err
	}
	return stackInfo.AuthContext, nil
}

// WithIsolatedIdentity authenticates, uses and releases a store's identities
// while preserving its root provider's reusable session. The private provider
// transaction covers the operation and cleanup, including concurrent callers.
func (m *manager) WithIsolatedIdentity(ctx context.Context, identityName string, operation func(context.Context, *schema.AuthContext) error) error {
	defer perf.Track(nil, "auth.Manager.WithIsolatedIdentity")()
	if operation == nil {
		return errUtils.ErrNilParam
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, release := signals.DeferExitForCleanup(ctx)
	defer release()
	chain, err := m.buildAuthenticationChain(identityName)
	if err != nil {
		return err
	}
	return m.credentialTransaction(ctx, chain[0], func(ctx context.Context) (result error) {
		isolated, stackInfo, err := m.newIsolatedManager()
		if err != nil {
			return err
		}
		defer func() {
			cleanup, cancel := signals.OwnedCleanupContext(ctx, 30*time.Second)
			defer cancel()
			result = errors.Join(result, isolated.releaseIsolatedIdentities(cleanup, chain), ctx.Err())
		}()
		// Retain custody of a bounded acquisition even after caller cancellation:
		// a server may already have minted credentials that still need logout.
		acquire, cancel := signals.OwnedCleanupContext(ctx, 45*time.Second)
		_, err = isolated.Authenticate(acquire, identityName)
		cancel()
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return operation(ctx, stackInfo.AuthContext)
	})
}

func (m *manager) releaseIsolatedIdentities(ctx context.Context, chain []string) error {
	defer perf.Track(nil, "auth.Manager.releaseIsolatedIdentities")()
	var result error
	for index := len(chain) - 1; index >= 0; index-- {
		if _, provider := m.providers[chain[index]]; provider {
			continue
		}
		result = errors.Join(result, m.Logout(ctx, chain[index], true))
	}
	processCredentialCache.Delete(m.chainCacheKey())
	return result
}

func (m *manager) newIsolatedManager() (*manager, *schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "auth.Manager.newIsolatedManager")()

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

	authManager, err := NewAuthManager(CopyGlobalAuthConfig(m.config), m.credentialStore, m.validator, isolatedStackInfo, m.cliConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}
	isolated, ok := authManager.(*manager)
	if !ok {
		return nil, nil, errUtils.ErrFailedToInitializeAuthManager
	}
	return isolated, isolatedStackInfo, nil
}
