package factory

import (
	emulatorIdentities "github.com/cloudposse/atmos/pkg/auth/identities/emulator"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterEmulatorIdentities registers all emulator-bound identity constructors with the factory.
// A single Identity type backs every emulator kind; the kind is carried on the config
// (set via the factory's ConfigSetter path after construction).
func RegisterEmulatorIdentities(f *Factory) {
	defer perf.Track(nil, "factory.RegisterEmulatorIdentities")()

	for _, kind := range types.EmulatorIdentityKinds {
		f.RegisterIdentity(kind, func(name string, _ map[string]any) (types.Identity, error) {
			defer perf.Track(nil, "factory.CreateEmulatorIdentity")()

			// The kind and emulator reference live on schema.Identity, injected via
			// SetConfig (ConfigSetter) by NewIdentity after construction. A minimal
			// placeholder config keeps Kind() valid until then.
			identity := &emulatorIdentities.Identity{}
			identity.SetName(name)
			return identity, nil
		})
	}
}
