package emulator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/container"
	emu "github.com/cloudposse/atmos/pkg/emulator"
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

// ResolveEmulator finds the running emulator selected by a project-scoped
// emulator identity, then loads its declaring stack's resolved spec and returns
// its connection profile. A qualified identity reference ("stack/name") maps
// directly to the INSTANCE shown by `atmos emulator list`; a bare name is accepted
// only when it identifies one running emulator globally.
func (emulatorResolver) ResolveEmulator(ctx context.Context, reference string) (map[string]string, []byte, error) {
	defer perf.Track(nil, "componentemulator.ResolveEmulator")()

	profile, err := resolveEmulatorProfile(ctx, reference)
	if err != nil {
		return nil, nil, err
	}
	return profile.Env, profile.Kubeconfig, nil
}

// resolveEmulatorProfile resolves the live profile for the running emulator
// selected by an identity reference. It is shared by auth identities and the
// Terraform provider contributor so they always target the same instance.
func resolveEmulatorProfile(ctx context.Context, reference string) (emu.Profile, error) {
	ref, err := parseEmulatorReference(reference)
	if err != nil {
		return emu.Profile{}, err
	}

	// A qualified reference is an exact configured component address. Resolve it
	// before consulting the runtime so an absent component is not misreported as
	// a stopped emulator.
	var configured *resolved
	if ref.Stack != "" {
		info := schema.ConfigAndStacksInfo{Stack: ref.Stack, ComponentFromArg: ref.Name, ComponentType: cfg.EmulatorComponentType}
		configured, err = prepare(&info)
		if err != nil {
			if errors.Is(err, errUtils.ErrInvalidComponent) {
				return emu.Profile{}, emulatorNotConfiguredError(ref)
			}
			return emu.Profile{}, err
		}
	}

	atmosConfig, err := initCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return emu.Profile{}, err
	}

	manager := newManager(strings.TrimSpace(atmosConfig.Container.Runtime.Provider), atmosConfig.Container.Runtime.AutoStart)
	if configured != nil {
		manager = configured.manager()
	}
	statuses, err := manager.Ps(ctx, "")
	if err != nil {
		return emu.Profile{}, err
	}

	matches := runningEmulatorsMatching(statuses, ref)
	switch len(matches) {
	case 0:
		return emu.Profile{}, emulatorNotRunningError(ref)
	case 1:
		// Continue below.
	default:
		addresses := make([]string, 0, len(matches))
		for _, match := range matches {
			addresses = append(addresses, emulatorInstanceAddress(match.Stack, match.Name))
		}
		return emu.Profile{}, fmt.Errorf(
			"%w: emulator %q matches multiple running instances (%s); set the identity's emulator to one INSTANCE value",
			errUtils.ErrEmulatorAmbiguous,
			ref.String(),
			strings.Join(addresses, ", "),
		)
	}

	r := configured
	if r == nil {
		info := schema.ConfigAndStacksInfo{Stack: matches[0].Stack, ComponentFromArg: ref.Name, ComponentType: cfg.EmulatorComponentType}
		r, err = prepare(&info)
		if err != nil {
			return emu.Profile{}, err
		}
	}

	_, profile, err := r.manager().Resolve(ctx, &r.spec, matches[0].Stack, ref.Name)
	if err != nil {
		return emu.Profile{}, err
	}
	return profile, nil
}

func emulatorNotConfiguredError(ref emulatorReference) error {
	builder := errUtils.Build(errUtils.ErrEmulatorNotConfigured).
		WithTitle("Emulator not configured").
		WithCausef("emulator %q is not configured", ref.String())

	if ref.Stack != "" {
		return builder.
			WithHintf("Configure emulator %q in stack %q before starting it.", ref.Name, ref.Stack).
			Err()
	}

	return builder.
		WithHint("Run `atmos emulator list` to find configured emulator instances.").
		Err()
}

func emulatorNotRunningError(ref emulatorReference) error {
	builder := errUtils.Build(errUtils.ErrEmulatorNotRunning).
		WithTitle("Emulator not running").
		WithCausef("emulator %q is not running", ref.String())

	if ref.Stack != "" {
		return builder.
			WithHintf("Start it with `atmos emulator up %s -s %s`.", ref.Name, ref.Stack).
			Err()
	}

	return builder.
		WithHint("Run `atmos emulator list` to find a configured emulator instance.").
		WithHintf("Start it with `atmos emulator up %s -s <stack>`.", ref.Name).
		Err()
}

type emulatorReference struct {
	Stack string
	Name  string
}

func parseEmulatorReference(value string) (emulatorReference, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return emulatorReference{}, fmt.Errorf("%w: emulator reference cannot be empty", errUtils.ErrEmulatorConfigInvalid)
	}

	stack, name, qualified := strings.Cut(value, "/")
	if !qualified {
		return emulatorReference{Name: stack}, nil
	}
	stack = strings.TrimSpace(stack)
	name = strings.TrimSpace(name)
	if stack == "" || name == "" {
		return emulatorReference{}, fmt.Errorf("%w: emulator reference %q must be `<stack>/<name>`", errUtils.ErrEmulatorConfigInvalid, value)
	}
	return emulatorReference{Stack: stack, Name: name}, nil
}

func (r emulatorReference) String() string {
	if r.Stack == "" {
		return r.Name
	}
	return emulatorInstanceAddress(r.Stack, r.Name)
}

func emulatorInstanceAddress(stack, name string) string {
	return stack + "/" + name
}

func runningEmulatorsMatching(statuses []emu.Status, reference emulatorReference) []emu.Status {
	matches := make([]emu.Status, 0, 1)
	for _, status := range statuses {
		if status.Name == reference.Name &&
			(reference.Stack == "" || status.Stack == reference.Stack) &&
			container.IsContainerRunning(status.Status) {
			matches = append(matches, status)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Stack != matches[j].Stack {
			return matches[i].Stack < matches[j].Stack
		}
		return matches[i].Container < matches[j].Container
	})
	return matches
}
