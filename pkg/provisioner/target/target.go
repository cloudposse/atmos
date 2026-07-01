package target

import (
	"context"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provisioner delivers a rendered ProvisionArtifact to a destination.
// Implementations are registered by kind (e.g. "git", "kubernetes") and must
// publish already-rendered files only — a Provisioner never renders manifests
// or runs `generate`.
type Provisioner interface {
	// Deliver publishes the artifact to the destination described by the input.
	Deliver(ctx context.Context, in *DeliverInput) error
}

// IdentityEnvironmentProvider supplies the composed identity environment for an
// Atmos Auth identity (including linked auto-provision integrations such as
// github/sts, which materializes GIT_CONFIG_* variables). The auth.AuthManager
// satisfies this interface. It is declared here (rather than imported from
// pkg/git) so the neutral registry stays free of service dependencies.
type IdentityEnvironmentProvider interface {
	EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error)
}

// DeliverInput carries everything a target provisioner needs to publish an
// artifact. TargetConfig is the merged `provision.targets.<name>` block for the
// selected target.
type DeliverInput struct {
	// AtmosConfig is the fully merged Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// TargetName is the configured name of the selected target.
	TargetName string
	// TargetConfig is the merged provision.targets.<name> block.
	TargetConfig map[string]any
	// Artifact is the rendered, producer-agnostic artifact to deliver.
	Artifact ProvisionArtifact
	// AuthContext is the resolved auth context for the component, if any.
	AuthContext *schema.AuthContext
	// EnvProvider composes the identity environment for targets that authenticate
	// via Atmos Auth (e.g. the git target's GitHub STS credentials). May be nil,
	// in which case ambient credentials apply.
	EnvProvider IdentityEnvironmentProvider
}

// registry is the singleton target-provisioner registry. It follows the Atmos
// registry pattern (see cmd/internal/registry.go): implementations self-register
// a singleton instance under their kind from init(), and Get returns that same
// instance. This is deliberately NOT a factory — no constructor func is stored.
type registry struct {
	mu           sync.RWMutex
	provisioners map[string]Provisioner
}

var globalRegistry = &registry{
	provisioners: make(map[string]Provisioner),
}

// Register adds a target provisioner instance under its kind. It is typically
// called during package initialization via init():
//
//	func init() { target.Register("git", &gitProvisioner{}) }
//
// Registering the same kind twice replaces the prior instance (last write wins),
// which keeps testing and future overrides simple.
func Register(kind string, p Provisioner) {
	defer perf.Track(nil, "target.Register")()

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.provisioners[kind] = p
}

// Get returns the registered provisioner instance for the kind. The boolean is
// false when no provisioner is registered for that kind.
func Get(kind string) (Provisioner, bool) {
	defer perf.Track(nil, "target.Get")()

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	p, ok := globalRegistry.provisioners[kind]
	return p, ok
}

// RegisteredKinds returns the sorted kinds currently registered.
func RegisteredKinds() []string {
	defer perf.Track(nil, "target.RegisteredKinds")()

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	kinds := make([]string, 0, len(globalRegistry.provisioners))
	for kind := range globalRegistry.provisioners {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// Deliver resolves the registered provisioner for the kind and delivers the
// artifact. It returns ErrProvisionTargetKindUnknown when no provisioner is
// registered for the kind.
func Deliver(ctx context.Context, kind string, in *DeliverInput) error {
	defer perf.Track(in.AtmosConfig, "target.Deliver")()

	p, ok := Get(kind)
	if !ok {
		return fmt.Errorf("%w: %q (registered: %v)", errUtils.ErrProvisionTargetKindUnknown, kind, RegisteredKinds())
	}
	return p.Deliver(ctx, in)
}
