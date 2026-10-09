package dependencies

import (
	"fmt"
	"maps"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultsRequest describes how the project's .tool-versions manifest is applied
// to one environment.
//
// The manifest is a baseline. Workflows install every usable entry. Everything
// else only installs the tools the run itself selects (a component's executable
// and its companions) when the manifest defines them; every other manifest tool
// is added to the environment only when it is already installed.
type defaultsRequest struct {
	// installAll installs every usable manifest entry that no explicit
	// dependency overrides. Only workflows set it.
	installAll bool
	// selected lists the bare executable names the run needs (for example
	// "terraform" or "tofu"). Empty for workflows, commands, and hooks.
	selected []string
}

// toolPlan is the result of overlaying the manifest on explicit dependencies.
type toolPlan struct {
	// install holds explicit dependencies plus selected manifest tools. These are
	// handed to the installer, which downloads any that are missing.
	install map[string]string
	// present holds manifest tools that are already installed. They only
	// contribute to PATH and resolved binaries and are never installed.
	present map[string]string
}

// newEnvironmentWithDefaults overlays resolved explicit dependencies on the
// project's tool versions. Defaults never constrain explicit dependencies, and
// they are best-effort: entries that cannot be used are skipped instead of
// aborting the run.
func newEnvironmentWithDefaults(
	atmosConfig *schema.AtmosConfiguration,
	explicit map[string]string,
	req defaultsRequest,
	opts ...envOption,
) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.newEnvironmentWithDefaults")()

	manifest, err := LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		return nil, err
	}

	cfg := &envConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	plan, err := planToolDefaults(manifest, explicit, req, newToolIdentity(atmosConfig, cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to overlay tool defaults: %w", err)
	}
	return newEnvironmentWithPresent(atmosConfig, plan.install, plan.present, opts...)
}

// planToolDefaults decides which manifest entries are installed, which are
// PATH-only, and which are dropped. Neither input map is mutated, including when
// the installer later resolves version ranges in the returned install map.
func planToolDefaults(manifest, explicit map[string]string, req defaultsRequest, ids *toolIdentity) (*toolPlan, error) {
	plan := &toolPlan{
		install: make(map[string]string, len(explicit)),
		present: map[string]string{},
	}
	maps.Copy(plan.install, explicit)

	usable := usableDefaults(manifest)
	// An explicit key always overrides the manifest entry with the same key.
	for tool := range explicit {
		delete(usable, tool)
	}
	if len(usable) == 0 {
		return plan, nil
	}

	overridden, err := explicitIdentities(explicit, ids)
	if err != nil {
		return nil, err
	}
	groups, err := groupDefaults(usable, overridden, ids)
	if err != nil {
		return nil, err
	}

	for i := range groups {
		group := &groups[i]
		switch {
		case req.installAll, ids.groupSelected(group, req.selected):
			plan.install[group.key()] = group.version
		case ids.installed(group):
			plan.present[group.key()] = group.version
		}
	}
	return plan, nil
}

// explicitIdentities resolves every explicit dependency to its owner/repo
// identity so manifest entries for the same tool can be dropped. Explicit
// dependencies stay strict: a resolution failure is an error.
func explicitIdentities(explicit map[string]string, ids *toolIdentity) (map[string]bool, error) {
	identities := make(map[string]bool, len(explicit))
	for tool := range explicit {
		id, err := ids.identity(tool)
		if err != nil {
			return nil, fmt.Errorf("resolve dependency %q: %w", tool, err)
		}
		identities[id] = true
	}
	return identities, nil
}
