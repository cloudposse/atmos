package emulator

import (
	"context"

	cfg "github.com/cloudposse/atmos/pkg/config"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	emu.RegisterProfileResolver(resolveProfile)
}

// resolveProfile is the prepare-backed ProfileResolver for the `!emulator` YAML
// function. It loads the emulator component's resolved spec for (currentStack, ref),
// discovers the running container, and returns its live endpoint + connection
// profile. Resolve harvests the kubeconfig into the profile for kubernetes targets, so
// `!emulator <k3s> kubeconfig` materializes without branching on the target here.
//
// It lives here (not in pkg/emulator) because it needs stack processing
// (internal/exec, via prepare) — and pkg/component/emulator already imports it,
// keeping pkg/emulator and internal/exec free of a cycle.
func resolveProfile(_ *schema.AtmosConfiguration, ref, currentStack string, _ *schema.ConfigAndStacksInfo) (*emu.Endpoint, *emu.Profile, error) {
	defer perf.Track(nil, "componentemulator.resolveProfile")()

	info := schema.ConfigAndStacksInfo{Stack: currentStack, ComponentFromArg: ref, ComponentType: cfg.EmulatorComponentType}
	r, err := prepare(&info)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	endpoint, profile, err := r.manager().Resolve(ctx, &r.spec, currentStack, ref)
	if err != nil {
		return nil, nil, err
	}

	return &endpoint, &profile, nil
}
