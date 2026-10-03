package secrets

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/utils"
)

// Service is the CRUD facade for the `atmos secret` CLI. It operates over declarations
// extracted from a component section and dispatches to the appropriate backend provider.
type Service struct {
	atmosConfig *schema.AtmosConfiguration
	// componentSection is the resolved component section holding secrets.vars declarations.
	componentSection map[string]any
	stack            string
	component        string
	// evaluator lazily resolves backend selectors (e.g. `!aws.cloudformation.output ...`). It is nil
	// for credential-free services, which report selector-backed declarations as unresolved.
	evaluator SelectorEvaluator
}

// NewService creates a Service scoped to a (stack, component) and its resolved component
// section (which carries the secrets.vars declarations after inheritance/merge).
func NewService(atmosConfig *schema.AtmosConfiguration, stack, component string, componentSection map[string]any, opts ...Option) *Service {
	defer perf.Track(atmosConfig, "secrets.NewService")()

	return &Service{
		atmosConfig:      atmosConfig,
		componentSection: componentSection,
		stack:            stack,
		component:        component,
		evaluator:        applyOptions(opts).evaluator,
	}
}

// provider resolves the backend provider for a declaration, lazily evaluating any backend selector
// for that declaration only.
func (s *Service) provider(decl *Declaration) (providers.Provider, error) {
	req := providerRequest{
		atmosConfig: s.atmosConfig,
		section:     s.componentSection,
		stack:       s.stack,
		component:   s.component,
		evaluator:   s.evaluator,
	}
	return req.provider(decl)
}

// Declarations returns the declared secrets for the service's scope, sorted by name.
func (s *Service) Declarations() []Declaration {
	defer perf.Track(s.atmosConfig, "secrets.Service.Declarations")()

	m := ExtractDeclarations(s.componentSection)
	out := make([]Declaration, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GenerableVault identifies a vault whose backend supports key generation (the registry-pattern
// keygen capability). Track is the backend track (e.g. "sops"); Name is the vault name.
type GenerableVault struct {
	Track string
	Name  string
}

// VaultsMissingKeys returns the vaults referenced by this scope's declarations whose backend
// supports key generation but has no key material yet, so callers can offer to generate one. It is
// backend-agnostic: any provider implementing providers.KeyGenerator participates. Results are
// de-duplicated and sorted by name.
func (s *Service) VaultsMissingKeys() ([]GenerableVault, error) {
	defer perf.Track(s.atmosConfig, "secrets.Service.VaultsMissingKeys")()

	seen := map[string]bool{}
	var missing []GenerableVault
	for _, decl := range s.Declarations() {
		d := decl
		key := string(d.BackendType) + "\x00" + d.BackendName
		if d.BackendName == "" || seen[key] {
			continue
		}
		seen[key] = true
		prov, err := s.provider(&d)
		if err != nil {
			// A vault whose name comes from an unresolvable selector cannot be keyed yet; skip it
			// rather than blocking key generation for the declarations that can be resolved.
			if errors.Is(err, ErrSelectorUnresolved) {
				continue
			}
			return nil, err
		}
		if kg, ok := prov.(providers.KeyGenerator); ok && !kg.HasKey() {
			missing = append(missing, GenerableVault{Track: string(d.BackendType), Name: d.BackendName})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Name < missing[j].Name })
	return missing, nil
}

// GenerateKeyForVault generates key material for a vault referenced by this scope and returns what
// the backend produced.
func (s *Service) GenerateKeyForVault(v GenerableVault) (*providers.KeygenResult, error) {
	defer perf.Track(s.atmosConfig, "secrets.Service.GenerateKeyForVault")()

	decl := Declaration{BackendType: BackendType(v.Track), BackendName: v.Name}
	prov, err := s.provider(&decl)
	if err != nil {
		return nil, err
	}
	kg, ok := prov.(providers.KeyGenerator)
	if !ok {
		return nil, fmt.Errorf("%w: vault %q", ErrKeygenUnsupported, v.Name)
	}
	return kg.GenerateKey(s.atmosConfig.BasePath)
}

// declarationFor returns the declaration for a name or an error if undeclared.
func (s *Service) declarationFor(name string) (Declaration, error) {
	decl, ok := LookupDeclaration(s.componentSection, name)
	if !ok {
		return Declaration{}, fmt.Errorf("%w: %q", ErrSecretNotDeclared, name)
	}
	if err := decl.ScopeConflict(); err != nil {
		return Declaration{}, err
	}
	return decl, nil
}

// providerAndCoord resolves the backend provider and coordinate for a declared secret.
func (s *Service) providerAndCoord(name string) (providers.Provider, providers.Coordinate, error) {
	decl, err := s.declarationFor(name)
	if err != nil {
		return nil, providers.Coordinate{}, err
	}
	provider, err := s.provider(&decl)
	if err != nil {
		if errors.Is(err, ErrSelectorUnresolved) {
			return nil, providers.Coordinate{}, err
		}
		return nil, providers.Coordinate{}, fmt.Errorf("%w (secret %q)", err, name)
	}
	coord := coordinateForDeclaration(&decl, s.stack, s.component)
	if err := checkScopeSupported(provider, &decl, coord); err != nil {
		return nil, providers.Coordinate{}, err
	}
	return provider, coord, nil
}

// checkScopeSupported rejects a declaration whose resolved scope its backend cannot represent,
// before any read or write (e.g. an instance-scoped secret on a backend that only scopes by
// environment). Returns ErrScopeUnsupported with actionable context.
func checkScopeSupported(provider providers.Provider, decl *Declaration, coord providers.Coordinate) error {
	if provider.SupportsScope(coord.Scope) {
		return nil
	}
	return errUtils.Build(providers.ErrScopeUnsupported).
		WithCausef("secret %q is %s-scoped but backend %q (%s) does not support that scope",
			decl.Name, coord.Scope, decl.BackendName, provider.Kind()).
		Err()
}

// Set stores a value for a declared secret.
func (s *Service) Set(name string, value any) error {
	defer perf.Track(s.atmosConfig, "secrets.Service.Set")()

	provider, coord, err := s.providerAndCoord(name)
	if err != nil {
		return err
	}
	return provider.Set(coord, value)
}

// Get retrieves a value for a declared secret, registering it with the masker.
func (s *Service) Get(name string, opts ResolveOptions) (any, error) {
	defer perf.Track(s.atmosConfig, "secrets.Service.Get")()

	provider, coord, err := s.providerAndCoord(name)
	if err != nil {
		return nil, err
	}

	value, err := provider.Get(coord)
	if err != nil {
		if opts.Default != nil {
			return *opts.Default, nil
		}
		// Build via the error builder (not multi-%w fmt.Errorf) so any actionable hints the
		// provider attached (e.g. how to initialize the file or supply the age key) are lifted
		// to the top-level error and rendered; errors.Is still matches ErrSecretMissing and the
		// provider's sentinels.
		return nil, errUtils.Build(ErrSecretMissing).WithCausef("%q: %w", name, err).Err()
	}

	if opts.Path != "" {
		value, err = utils.EvaluateYqExpression(s.atmosConfig, value, opts.Path)
		if err != nil {
			return nil, err
		}
	}

	io.RegisterSecretValue(value)
	return value, nil
}

// Delete removes a declared secret's value from its backend.
func (s *Service) Delete(name string) error {
	defer perf.Track(s.atmosConfig, "secrets.Service.Delete")()

	provider, coord, err := s.providerAndCoord(name)
	if err != nil {
		return err
	}
	return provider.Delete(coord)
}

// DeleteAll removes every declared secret's value from its backend, returning the number of
// declarations processed. Delete is idempotent (it no-ops on values that are not initialized),
// so this also serves as a clean "reset" of a SOPS file's declared keys.
func (s *Service) DeleteAll() (int, error) {
	defer perf.Track(s.atmosConfig, "secrets.Service.DeleteAll")()

	decls := s.Declarations()
	var errs []error
	deleted := 0
	for _, decl := range decls {
		// One declaration whose backend cannot be resolved (for example a selector whose producer
		// was deleted) must not strand the values of the declarations that can still be deleted.
		if err := s.Delete(decl.Name); err != nil {
			errs = append(errs, err)
			continue
		}
		deleted++
	}
	if len(errs) > 0 {
		return deleted, errors.Join(errs...)
	}
	return deleted, nil
}

// Reset overwrites a file-based provider's backing file (e.g. SOPS) with a clean, empty document
// for this scope, creating it if missing. It is a no-op for backends that are not file-based
// (they have no whole-file state to reset). Returns whether any provider was reset.
func (s *Service) Reset() (bool, error) {
	defer perf.Track(s.atmosConfig, "secrets.Service.Reset")()

	seen := make(map[string]bool)
	didReset := false
	for _, decl := range s.Declarations() {
		provider, coord, err := s.providerAndCoord(decl.Name)
		if err != nil {
			return didReset, err
		}
		resettable, ok := provider.(providers.FileResettable)
		if !ok {
			continue
		}
		// Reset each distinct backing provider once (multiple secrets may share one file).
		key := fmt.Sprintf("%s/%s", coord.Stack, coord.Component)
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := resettable.Reset(coord); err != nil {
			return didReset, err
		}
		didReset = true
	}
	return didReset, nil
}

// Status reports whether each declared secret is initialized in its backend. It never
// registers values with the masker (uses the backend status check, not Get) and never decrypts.
//
// Listing is credential-free by default. A provider whose existence check is local (no network,
// no auth, no decryption — e.g. SOPS, which reads cleartext key names) is always checked. A
// non-local provider (a remote store) is checked only when verify is true; otherwise its status
// is reported as Unknown so that `atmos secret list` needs no authenticated identity. Callers
// that require an authoritative answer for remote backends (e.g. `secret validate`, or
// `secret list --verify`) pass verify=true and must have wired the store auth resolver.
func (s *Service) Status(verify bool) []Status {
	defer perf.Track(s.atmosConfig, "secrets.Service.Status")()

	decls := s.Declarations()
	out := make([]Status, 0, len(decls))
	for _, decl := range decls {
		st := Status{
			Declaration: decl,
			Coordinate:  coordinateForDeclaration(&decl, s.stack, s.component),
		}
		if err := decl.ScopeConflict(); err != nil {
			st.Err = err
			st.Reason = err.Error()
			out = append(out, st)
			continue
		}
		// Without --verify there is no authenticated scope to evaluate a backend selector in, so
		// report an explicit unresolved state with its reason instead of a bare error.
		if !verify && IsSelector(decl.BackendName) {
			st.Unknown = true
			st.Unresolved = true
			st.Reason = fmt.Sprintf("backend selector %q is resolved with --verify (requires credentials)", strings.TrimSpace(decl.BackendName))
			out = append(out, st)
			continue
		}
		provider, err := s.provider(&decl)
		if err != nil {
			st.Err = err
			st.Reason = err.Error()
			out = append(out, st)
			continue
		}
		if err := checkScopeSupported(provider, &decl, st.Coordinate); err != nil {
			st.Err = err
			st.Reason = err.Error()
			out = append(out, st)
			continue
		}
		// Skip the existence check for non-local backends unless verification was requested:
		// contacting a remote store needs network + credentials, which listing avoids by default.
		if !verify && !providerStatusIsLocal(provider) {
			st.Unknown = true
			out = append(out, st)
			continue
		}
		initialized, err := provider.Status(st.Coordinate)
		st.Initialized = initialized
		st.Err = err
		if err != nil {
			st.Reason = err.Error()
		}
		out = append(out, st)
	}
	return out
}

// providerStatusIsLocal reports whether the provider's existence check is credential-free
// (LocalStatus capability). Providers that do not implement the capability are treated as remote.
func providerStatusIsLocal(provider providers.Provider) bool {
	ls, ok := provider.(providers.LocalStatus)
	return ok && ls.LocalStatusCheck()
}

// ScopeOf returns the resolved scope of a declared secret and whether it is declared. An undeclared
// name returns ("", false). A declaration with no explicit scope defaults to ScopeInstance.
func (s *Service) ScopeOf(name string) (Scope, bool) {
	defer perf.Track(s.atmosConfig, "secrets.Service.ScopeOf")()

	decl, ok := LookupDeclaration(s.componentSection, name)
	if !ok {
		return "", false
	}
	if decl.Scope == "" {
		return ScopeInstance, true
	}
	return decl.Scope, true
}

// IsDeclared reports whether a key is declared as a secret in this scope.
func (s *Service) IsDeclared(name string) bool {
	defer perf.Track(s.atmosConfig, "secrets.Service.IsDeclared")()

	_, ok := LookupDeclaration(s.componentSection, name)
	return ok
}
