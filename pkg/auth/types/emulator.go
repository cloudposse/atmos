package types

import "context"

// EmulatorResolver resolves a running emulator's connection profile for an
// identity that targets it (kind: <target>/emulator). It is implemented ABOVE the
// auth layer (it needs stack processing and the container runtime) and injected
// into identities at auth-manager construction. The auth package never imports the
// emulator implementation, avoiding an import cycle.
type EmulatorResolver interface {
	// ResolveEmulator returns the SDK environment variables and/or a kubeconfig
	// for the named emulator in the given stack. Returns an error if the emulator
	// is not declared in the stack or is not running.
	ResolveEmulator(ctx context.Context, stack, name string) (env map[string]string, kubeconfig []byte, err error)
}
