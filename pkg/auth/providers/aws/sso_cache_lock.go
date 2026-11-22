package aws

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/filelock"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// ssoTokenCacheLockTimeout bounds how long a process waits for another
// process that is reading or refreshing the same SSO session token.
const ssoTokenCacheLockTimeout = 30 * time.Second

// loadOrRefreshDiskToken returns a usable token from the on-disk session cache,
// refreshing an expired one with its refresh token when possible. It reports
// false when the caller must run the device-authorization flow.
//
// Several Atmos processes (for example parallel kubectl exec-plugin calls) can
// find the same expired token at once. AWS rotates the refresh token on each
// exchange, so a second exchange of the same token fails and would send that
// process to the interactive device flow. Holding a cross-process lock while
// reading and refreshing makes the second process wait and then read the token
// the first one wrote.
func (p *ssoProvider) loadOrRefreshDiskToken(ctx context.Context) (ssoTokenCache, bool, error) {
	var (
		token     ssoTokenCache
		found     bool
		clientErr error
	)

	p.withTokenCacheLock(ctx, func() error {
		// 2. On-disk cache. Survives process restart; populated by prior atmos sessions.
		if diskCached, ok := p.loadFullCachedToken(); ok {
			log.Debug("Using on-disk cached SSO token", "expiresAt", diskCached.ExpiresAt)
			token, found = diskCached, true
			return nil
		}

		// 3. Refresh-token exchange. Skipped if the on-disk cache had no refresh token
		//    (e.g., written by an older atmos version, or registration expired).
		expired, ok := p.loadExpiredCachedTokenForRefresh()
		if !ok {
			return nil
		}
		oidcClient, err := p.newOIDCClient(ctx)
		if err != nil {
			clientErr = err
			return nil
		}
		refreshed, rerr := tryRefreshToken(ctx, oidcClient, expired)
		if rerr != nil {
			log.Debug("Refresh-token exchange failed; falling through to device authorization", "error", rerr)
			return nil
		}
		log.Debug("SSO token refreshed without browser interaction")
		if serr := p.saveFullCachedToken(refreshed); serr != nil {
			log.Debug("Failed to persist refreshed token", "error", serr)
		}
		token, found = refreshed, true
		return nil
	})

	return token, found, clientErr
}

// withTokenCacheLock runs fn while holding a cross-process lock on this
// provider's session cache file. The lock is best effort: when it cannot be
// acquired (no cache directory, timeout), fn runs without it, which is no worse
// than the unlocked behavior.
func (p *ssoProvider) withTokenCacheLock(ctx context.Context, fn func() error) {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		_ = fn()
		return
	}

	lockCtx, cancel := context.WithTimeout(ctx, ssoTokenCacheLockTimeout)
	defer cancel()

	ran := false
	lockErr := filelock.New(tokenPath+".lock").WithExclusive(lockCtx, func() error {
		ran = true
		return fn()
	})
	if !ran {
		log.Debug("Could not lock the SSO token cache; continuing without the lock", "path", tokenPath, "error", lockErr)
		_ = fn()
	}
}
