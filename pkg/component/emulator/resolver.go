package emulator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// emulatorResolver implements auth's EmulatorResolver. It lives here (not in
// pkg/auth or pkg/emulator) because it needs both stack processing (internal/exec,
// via prepare) and the emulator manager — and pkg/component/emulator already
// imports both, so registering it keeps pkg/auth cycle-free.
type emulatorResolver struct{}

func init() {
	auth.SetEmulatorResolver(emulatorResolver{})
}

// ResolveEmulator loads the emulator component's resolved spec for (stack, name),
// then returns its connection profile: SDK env vars for cloud targets, or a harvested
// kubeconfig for the kubernetes target. Resolve populates whichever the target uses, so
// this returns both without branching on the target itself.
func (emulatorResolver) ResolveEmulator(ctx context.Context, stack, name string) (map[string]string, []byte, error) {
	defer perf.Track(nil, "componentemulator.ResolveEmulator")()

	info := schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: name, ComponentType: cfg.EmulatorComponentType}
	r, err := prepare(&info)
	if err != nil {
		return nil, nil, err
	}

	_, profile, err := r.manager().Resolve(ctx, &r.spec, stack, name)
	if err != nil {
		return nil, nil, err
	}
	return profile.Env, profile.Kubeconfig, nil
}
