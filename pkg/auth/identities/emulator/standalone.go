package emulator

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// IsStandalone reports that emulator-bound identities authenticate without an upstream
// provider step: the running emulator container is the credential source. Part of the
// types.StandaloneIdentity interface the chain manager dispatches through.
func (i *Identity) IsStandalone() bool { return true }

// AuthenticateStandalone authenticates a standalone emulator identity. Emulator
// identities do not mint credentials (the connection profile is injected at
// environment-preparation time), so this returns nil credentials — mirroring the
// standalone ambient identity. Part of the types.StandaloneIdentity interface.
func (i *Identity) AuthenticateStandalone(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "emulator.Identity.AuthenticateStandalone")()

	credentials, err := i.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: emulator identity %q authentication failed: %w", errUtils.ErrAuthenticationFailed, i.Name(), err)
	}

	log.Debug("Emulator identity authenticated (no-op)", logKeyIdentity, i.Name())
	return credentials, nil
}
