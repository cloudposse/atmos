// Package emulator implements the emulator-bound auth identities
// (kind: aws/emulator, gcp/emulator, azure/emulator, kubernetes/emulator).
//
// These are root identities not minted by a cloud: the running emulator container
// is the credential source. They authenticate to nothing (like the ambient
// identity) and inject their connection profile at environment-preparation time —
// SDK env vars for the cloud targets, or a harvested kubeconfig (written to a
// realm-scoped file, exported as KUBECONFIG) for the kubernetes target.
//
// The actual resolution is delegated to an injected types.EmulatorResolver, which
// lives above the auth layer (it needs stack processing and the container runtime).
// This keeps pkg/auth free of any emulator/stack-processing import (no cycle).
package emulator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	logKeyIdentity = "identity"

	// Subdirectory (under the XDG config dir) holding harvested kubeconfigs.
	kubeconfigSubdir = "emulator"

	// Permission for created credential directories.
	permDir os.FileMode = 0o700
	// Permission for the written kubeconfig (contains the admin credential).
	permKubeconfig os.FileMode = 0o600

	// Standard kubeconfig environment variable.
	envKubeconfig = "KUBECONFIG"
)

// Identity implements an emulator-bound auth identity. A single type backs all four
// emulator kinds; behavior diverges only on the resolved profile shape (env vs kubeconfig).
type Identity struct {
	name     string
	realm    string
	config   *schema.Identity
	resolver types.EmulatorResolver
	stack    string
}

// New creates a new emulator identity for the given kind.
func New(name string, config *schema.Identity) (*Identity, error) {
	defer perf.Track(nil, "emulator.New")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if !types.IsEmulatorIdentityKind(config.Kind) {
		return nil, fmt.Errorf("%w: invalid identity kind for emulator: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
	return &Identity{name: name, config: config}, nil
}

// SetName sets the identity name.
func (i *Identity) SetName(name string) { i.name = name }

// SetConfig sets the identity configuration (invoked by the factory after construction).
func (i *Identity) SetConfig(config *schema.Identity) { i.config = config }

// SetRealm sets the credential isolation realm for filesystem paths.
func (i *Identity) SetRealm(realm string) { i.realm = realm }

// SetEmulatorResolver injects the process-wide emulator resolver. Part of the
// emulatorResolverReceiver interface the auth manager duck-types against.
func (i *Identity) SetEmulatorResolver(resolver types.EmulatorResolver) { i.resolver = resolver }

// SetStack injects the stack the command runs in, used to scope the bare emulator
// name to a concrete address. Part of the emulatorResolverReceiver interface.
func (i *Identity) SetStack(stack string) { i.stack = stack }

// Kind returns the identity kind.
func (i *Identity) Kind() string {
	if i.config != nil {
		return i.config.Kind
	}
	return ""
}

// Name returns the identity name (falling back to the kind).
func (i *Identity) Name() string {
	if i.name != "" {
		return i.name
	}
	return i.Kind()
}

// GetProviderName returns this identity's own name. Emulator identities are root
// identities (no upstream provider); the chain is just the identity itself.
func (i *Identity) GetProviderName() (string, error) {
	return i.Name(), nil
}

// Validate validates the identity configuration.
func (i *Identity) Validate() error {
	defer perf.Track(nil, "emulator.Identity.Validate")()

	if i.config == nil {
		return fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	if !types.IsEmulatorIdentityKind(i.config.Kind) {
		return fmt.Errorf("%w: invalid identity kind for emulator: %s", errUtils.ErrInvalidIdentityKind, i.config.Kind)
	}
	if i.config.Via != nil {
		return fmt.Errorf("%w: emulator identity %q must not define via", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	if strings.TrimSpace(i.config.Emulator) == "" {
		return fmt.Errorf("%w: emulator identity %q must set `emulator: <name>`", errUtils.ErrInvalidIdentityConfig, i.Name())
	}
	return nil
}

// Authenticate is a no-op for emulator identities. Like the ambient identity, they
// do not mint or cache credentials — the emulator container is the live source, and
// the profile is resolved fresh at environment-preparation time.
func (i *Identity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "emulator.Identity.Authenticate")()

	log.Debug("Emulator identity authentication is a no-op", logKeyIdentity, i.Name())
	return nil, nil
}

// Environment returns no static environment variables. The profile is dynamic
// (it depends on a running emulator's live host port) and is resolved in
// PrepareEnvironment, which has the context needed to reach the resolver.
func (i *Identity) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns no credential paths. The kubernetes kubeconfig path is managed
// internally (written/removed by PrepareEnvironment/Logout).
func (i *Identity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment resolves the bound emulator's connection profile and merges it
// into the environment: SDK env vars for cloud targets, or a harvested kubeconfig
// (written to a realm-scoped file and exported as KUBECONFIG) for the kubernetes target.
func (i *Identity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "emulator.Identity.PrepareEnvironment")()

	out := make(map[string]string, len(environ))
	for k, v := range environ {
		out[k] = v
	}

	if err := i.Validate(); err != nil {
		return nil, err
	}
	if i.resolver == nil {
		return nil, fmt.Errorf("%w: identity %q cannot resolve emulator %q (emulator component not loaded)",
			errUtils.ErrEmulatorResolverUnavailable, i.Name(), i.config.Emulator)
	}

	env, kubeconfig, err := i.resolver.ResolveEmulator(ctx, i.stack, i.config.Emulator)
	if err != nil {
		return nil, fmt.Errorf("resolve emulator %q for identity %q: %w", i.config.Emulator, i.Name(), err)
	}

	for k, v := range env {
		out[k] = v
	}

	if len(kubeconfig) > 0 {
		path, writeErr := i.writeKubeconfig(kubeconfig)
		if writeErr != nil {
			return nil, writeErr
		}
		out[envKubeconfig] = appendPathList(out[envKubeconfig], path)
	}

	return out, nil
}

// PostAuthenticate populates the in-process auth context for cloud-target emulator
// identities. Stores, `!store`, store hooks, and the secrets engine build cloud SDK
// clients inside the atmos process and read credentials from the auth context (not
// the subprocess env that PrepareEnvironment sets for Terraform). Kubernetes is
// consumed via KUBECONFIG, so it needs nothing here.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "emulator.Identity.PostAuthenticate")()

	if params == nil || params.AuthContext == nil || i.resolver == nil || i.config == nil {
		return nil
	}
	if i.config.Kind == types.IdentityKindAWSEmulator {
		return i.setAWSAuthContext(ctx, params)
	}
	// TODO(emulator): populate the GCP/Azure auth contexts for gcp/azure emulator
	// identities so their in-process stores work too (the subprocess/Terraform path
	// already works for every target via PrepareEnvironment).
	return nil
}

// Logout removes the harvested kubeconfig file (best-effort). No-op for cloud targets.
func (i *Identity) Logout(_ context.Context) error {
	defer perf.Track(nil, "emulator.Identity.Logout")()

	path, err := i.kubeconfigPath()
	if err != nil {
		// Best-effort cleanup: a path we cannot compute has nothing to remove.
		return nil
	}
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		log.Debug("Failed to remove emulator kubeconfig", logKeyIdentity, i.Name(), "path", path, "error", removeErr)
	}
	return nil
}

// CredentialsExist reports true: the emulator is the live credential source.
func (i *Identity) CredentialsExist() (bool, error) { return true, nil }

// LoadCredentials returns nil: emulator identities have no stored credentials.
func (i *Identity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

// kubeconfigPath returns the realm-scoped path where this identity's kubeconfig is written.
func (i *Identity) kubeconfigPath() (string, error) {
	base, err := xdg.GetXDGConfigDir(filepath.Join(i.realm, kubeconfigSubdir), permDir)
	if err != nil {
		return "", fmt.Errorf("%w: resolve emulator kubeconfig dir: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return filepath.Join(base, i.Name()+".kubeconfig"), nil
}

// writeKubeconfig writes the harvested kubeconfig to a realm-scoped file and returns its path.
func (i *Identity) writeKubeconfig(data []byte) (string, error) {
	path, err := i.kubeconfigPath()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, permKubeconfig); err != nil {
		return "", fmt.Errorf("%w: write emulator kubeconfig: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return path, nil
}

// appendPathList appends newPath to an OS-path-list-separated value, deduplicating.
func appendPathList(existing, newPath string) string {
	if existing == "" {
		return newPath
	}
	if newPath == "" {
		return existing
	}
	sep := string(os.PathListSeparator)
	for _, part := range strings.Split(existing, sep) {
		if part == newPath {
			return existing
		}
	}
	return existing + sep + newPath
}
