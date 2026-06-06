package secrets

import (
	"fmt"
	"sort"

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
}

// NewService creates a Service scoped to a (stack, component) and its resolved component
// section (which carries the secrets.vars declarations after inheritance/merge).
func NewService(atmosConfig *schema.AtmosConfiguration, stack, component string, componentSection map[string]any) *Service {
	defer perf.Track(atmosConfig, "secrets.NewService")()

	return &Service{
		atmosConfig:      atmosConfig,
		componentSection: componentSection,
		stack:            stack,
		component:        component,
	}
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

// declarationFor returns the declaration for a name or an error if undeclared.
func (s *Service) declarationFor(name string) (Declaration, error) {
	decl, ok := LookupDeclaration(s.componentSection, name)
	if !ok {
		return Declaration{}, fmt.Errorf("%w: %q", ErrSecretNotDeclared, name)
	}
	return decl, nil
}

// providerAndCoord resolves the backend provider and coordinate for a declared secret.
func (s *Service) providerAndCoord(name string) (providers.Provider, providers.Coordinate, error) {
	decl, err := s.declarationFor(name)
	if err != nil {
		return nil, providers.Coordinate{}, err
	}
	provider, err := providerFor(s.atmosConfig, &decl, s.componentSection)
	if err != nil {
		return nil, providers.Coordinate{}, fmt.Errorf("%w (secret %q)", err, name)
	}
	return provider, coordinateForDeclaration(&decl, s.stack, s.component), nil
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
	for _, decl := range decls {
		if err := s.Delete(decl.Name); err != nil {
			return 0, err
		}
	}
	return len(decls), nil
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
// registers values with the masker (uses the backend status check, not Get).
func (s *Service) Status() []Status {
	defer perf.Track(s.atmosConfig, "secrets.Service.Status")()

	decls := s.Declarations()
	out := make([]Status, 0, len(decls))
	for _, decl := range decls {
		st := Status{
			Declaration: decl,
			Coordinate:  coordinateForDeclaration(&decl, s.stack, s.component),
		}
		provider, err := providerFor(s.atmosConfig, &decl, s.componentSection)
		if err != nil {
			st.Err = err
			out = append(out, st)
			continue
		}
		initialized, err := provider.Status(st.Coordinate)
		st.Initialized = initialized
		st.Err = err
		out = append(out, st)
	}
	return out
}

// IsDeclared reports whether a key is declared as a secret in this scope.
func (s *Service) IsDeclared(name string) bool {
	defer perf.Track(s.atmosConfig, "secrets.Service.IsDeclared")()

	_, ok := LookupDeclaration(s.componentSection, name)
	return ok
}
