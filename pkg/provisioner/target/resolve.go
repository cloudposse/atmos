package target

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// KindKubernetes delivers rendered manifests to a Kubernetes cluster.
	KindKubernetes = "kubernetes"
	// KindGit publishes rendered files to a Git deployment repository.
	KindGit = "git"

	// DefaultClusterTargetName is the implicit target used when no target is
	// selected and none is configured: it preserves the historical behavior of
	// applying directly to the cluster, so components without provision.targets
	// keep working with zero configuration.
	DefaultClusterTargetName = "cluster"

	provisionTargetsKey = "targets"
	provisionDefaultKey = "default"
	targetKindKey       = "kind"
)

// SelectedTarget is the resolved provision target for a delivery: its name,
// kind, and merged config block.
type SelectedTarget struct {
	// Name is the configured (or implicit) target name.
	Name string
	// Kind is the registered target kind (e.g. "git", "kubernetes").
	Kind string
	// Config is the merged provision.targets.<name> block (nil for the implicit cluster).
	Config map[string]any
}

// SelectTarget resolves which provision target to deliver to. Precedence:
// flagTarget, then provision.default, then the implicit "cluster" target.
//
// When no targets are configured (or the implicit "cluster" target is selected
// without an explicit definition), it returns the implicit Kubernetes cluster
// target so existing components apply to the cluster unchanged. An explicitly
// requested name that is not configured returns ErrProvisionTargetNotFound, and
// a configured target missing a kind returns ErrProvisionTargetKindMissing.
func SelectTarget(provisionSection map[string]any, flagTarget string) (*SelectedTarget, error) {
	defer perf.Track(nil, "target.SelectTarget")()

	return SelectTargetWithDefault(provisionSection, flagTarget, DefaultClusterTargetName, KindKubernetes)
}

// SelectTargetWithDefault is SelectTarget generalized over the implicit
// default target's name/kind, for component types whose zero-config delivery
// isn't the Kubernetes cluster (e.g. aws/cloudformation's implicit
// `kind: aws/cloudformation` direct-deploy target). SelectTarget is the
// Kubernetes/Helm-flavored convenience wrapper around this.
func SelectTargetWithDefault(provisionSection map[string]any, flagTarget, defaultName, defaultKind string) (*SelectedTarget, error) {
	defer perf.Track(nil, "target.SelectTargetWithDefault")()

	targets := targetsFromSection(provisionSection)

	name := selectedTargetNameWithDefault(provisionSection, flagTarget, defaultName)

	cfg, found := targets[name]
	if !found {
		if name == defaultName {
			// Implicit delivery: no explicit definition required.
			return &SelectedTarget{Name: defaultName, Kind: defaultKind}, nil
		}
		return nil, fmt.Errorf("%w: %q (configured: %v)", errUtils.ErrProvisionTargetNotFound, name, configuredTargetNames(targets))
	}

	kind, _ := cfg[targetKindKey].(string)
	if kind == "" {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrProvisionTargetKindMissing, name)
	}

	return &SelectedTarget{Name: name, Kind: kind, Config: cfg}, nil
}

// selectedTargetNameWithDefault applies the flag > provision.default > implicit
// default precedence.
func selectedTargetNameWithDefault(provisionSection map[string]any, flagTarget, defaultName string) string {
	if flagTarget != "" {
		return flagTarget
	}
	if provisionSection != nil {
		if def, ok := provisionSection[provisionDefaultKey].(string); ok && def != "" {
			return def
		}
	}
	return defaultName
}

// targetsFromSection extracts the provision.targets map as name -> config blocks.
func targetsFromSection(provisionSection map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any)
	if provisionSection == nil {
		return out
	}
	raw, ok := provisionSection[provisionTargetsKey].(map[string]any)
	if !ok {
		return out
	}
	for targetName, value := range raw {
		if block, ok := value.(map[string]any); ok {
			out[targetName] = block
		}
	}
	return out
}

// configuredTargetNames returns the configured target names for error context.
func configuredTargetNames(targets map[string]map[string]any) []string {
	names := make([]string, 0, len(targets))
	for targetName := range targets {
		names = append(names, targetName)
	}
	return names
}
