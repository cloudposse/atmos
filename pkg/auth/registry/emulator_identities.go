package registry

import (
	emulatorIdentities "github.com/cloudposse/atmos/pkg/auth/identities/emulator"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterEmulatorIdentities registers all emulator-bound identity constructors with the registry.
// A single Identity type backs every emulator kind; the kind is carried on the config
// (set via the registry's ConfigSetter path after construction).
func RegisterEmulatorIdentities(f *Registry) error {
	defer perf.Track(nil, "registry.RegisterEmulatorIdentities")()

	for _, kind := range types.EmulatorIdentityKinds {
		if err := f.RegisterIdentity(kind, func(name string, _ map[string]any) (types.Identity, error) {
			defer perf.Track(nil, "registry.CreateEmulatorIdentity")()

			// The kind and emulator reference live on schema.Identity, injected via
			// SetConfig (ConfigSetter) by NewIdentity after construction. A minimal
			// placeholder config keeps Kind() valid until then.
			identity := &emulatorIdentities.Identity{}
			identity.SetName(name)
			return identity, nil
		}); err != nil {
			return err
		}
	}
	return nil
}
