package dependencies

import (
	"fmt"
	"maps"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultsRequest describes the run an environment is built for. The
// toolchain.install policy decides how the project's .tool-versions manifest is
// applied to it (see manifestUseFor).
type defaultsRequest struct {
	// workflow marks a workflow run. Workflows declare every manifest tool, so
	// they install all of it under the declared and auto policies.
	workflow bool
	// selected lists the bare executable names the run needs (for example
	// "terraform" or "tofu"). Empty for workflows, commands, and hooks.
	selected []string
}

// manifestUse says what a run does with the .tool-versions manifest.
type manifestUse int

const (
	// The manifestIgnored use does not read the manifest at all.
	manifestIgnored manifestUse = iota
	// The manifestPresentOnly use adds manifest tools that are already installed
	// and never installs any.
	manifestPresentOnly
	// The manifestSelected use installs the manifest tools the run selects and
	// adds the other manifest tools only when they are already installed.
	manifestSelected
	// The manifestInstallAll use installs every usable manifest entry.
	manifestInstallAll
)

// planRequest is a defaultsRequest after the install policy has been applied.
type planRequest struct {
	use      manifestUse
	selected []string
}

// toolPlan is the result of overlaying the manifest on explicit dependencies.
type toolPlan struct {
	// install holds explicit dependencies plus manifest tools the policy installs.
	// These are handed to the installer, which downloads any that are missing.
	install map[string]string
	// present holds manifest tools that are already installed. They only
	// contribute to PATH and resolved binaries and are never installed.
	present map[string]string
}

// manifestUseFor maps the install policy and run kind to the manifest handling.
//
//	never:    manifest tools are PATH-only; nothing is installed.
//	declared: workflows install every manifest tool; every other run ignores the manifest.
//	auto:     workflows install every manifest tool; other runs install what they select.
//	always:   every run installs every manifest tool.
func manifestUseFor(policy schema.ToolchainInstall, workflow bool) manifestUse {
	switch policy {
	case schema.ToolchainInstallNever:
		return manifestPresentOnly
	case schema.ToolchainInstallAlways:
		return manifestInstallAll
	case schema.ToolchainInstallDeclared:
		if workflow {
			return manifestInstallAll
		}
		return manifestIgnored
	default:
		if workflow {
			return manifestInstallAll
		}
		return manifestSelected
	}
}

// newEnvironmentWithDefaults overlays resolved explicit dependencies on the
// project's tool versions according to toolchain.install. Defaults never
// constrain explicit dependencies, and they are best-effort: entries that cannot
// be used are skipped instead of aborting the run.
func newEnvironmentWithDefaults(
	atmosConfig *schema.AtmosConfiguration,
	explicit map[string]string,
	req defaultsRequest,
	opts ...envOption,
) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.newEnvironmentWithDefaults")()

	policy, err := InstallPolicy(atmosConfig)
	if err != nil {
		return nil, err
	}
	use := manifestUseFor(policy, req.workflow)

	var manifest map[string]string
	if use != manifestIgnored {
		manifest, err = LoadToolVersionsDependencies(atmosConfig)
		if err != nil {
			return nil, err
		}
	}

	cfg := &envConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	ids := newToolIdentity(atmosConfig, cfg)
	plan, err := planToolDefaults(manifest, explicit, planRequest{use: use, selected: req.selected}, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to overlay tool defaults: %w", err)
	}
	if policy == schema.ToolchainInstallNever {
		if err := requireInstalled(plan, ids); err != nil {
			return nil, err
		}
	}
	return newEnvironmentWithPresent(atmosConfig, plan.install, plan.present, opts...)
}

// planToolDefaults decides which manifest entries are installed, which are
// PATH-only, and which are dropped. Neither input map is mutated, including when
// the installer later resolves version ranges in the returned install map.
func planToolDefaults(manifest, explicit map[string]string, req planRequest, ids *toolIdentity) (*toolPlan, error) {
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
		if req.use == manifestInstallAll || (req.use == manifestSelected && ids.groupSelected(group, req.selected)) {
			plan.install[group.key()] = group.version
			continue
		}
		if version, ok := ids.installedVersion(group.identity, group.version); ok {
			plan.present[group.key()] = version
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
