package auth

import "github.com/cloudposse/atmos/pkg/auth/types"

// defaultEmulatorResolver is the process-wide emulator resolver, registered by
// pkg/component/emulator at init. It is nil in contexts that never import the
// emulator component (identities that target an emulator then fail with a clear
// error).
var defaultEmulatorResolver types.EmulatorResolver

// SetEmulatorResolver registers the process-wide emulator resolver used by
// identities that target an emulator (kind: <target>/emulator). Called at init by
// pkg/component/emulator; keeps pkg/auth free of any emulator/stack-processing
// import (no cycle).
func SetEmulatorResolver(resolver types.EmulatorResolver) {
	defaultEmulatorResolver = resolver
}

// emulatorResolverReceiver is the optional interface implemented by identities
// that resolve a running emulator's connection profile. The auth manager injects
// the resolver and the current stack the same way it injects the credential store.
type emulatorResolverReceiver interface {
	SetEmulatorResolver(resolver types.EmulatorResolver)
	SetStack(stack string)
}
