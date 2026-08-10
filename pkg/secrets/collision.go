package secrets

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// SopsPlacement is a single SOPS-backed secret's resolved file location within a scope. It is the
// input to the collision lookup that guards the advanced `spec.file` template path (the derive-in-
// code default is collision-safe by construction, so it never produces violations here).
type SopsPlacement struct {
	Stack     string
	Component string
	Secret    string //nolint:gosec // holds the secret's NAME (identifier), never its value.
	Scope     Scope
	File      string
}

// SopsPlacements returns the resolved SOPS file placement for each SOPS-backed declared secret in
// this service's scope. Non-SOPS (store-backed) secrets and unresolvable providers are skipped.
func (s *Service) SopsPlacements() []SopsPlacement {
	defer perf.Track(s.atmosConfig, "secrets.Service.SopsPlacements")()

	var out []SopsPlacement
	for _, decl := range s.Declarations() {
		d := decl
		if d.BackendType != BackendSops {
			continue
		}
		provider, err := providerFor(s.atmosConfig, &d, s.componentSection)
		if err != nil {
			continue
		}
		fp, ok := provider.(providers.FilePathProvider)
		if !ok {
			continue
		}
		coord := coordinateForDeclaration(&d, s.stack, s.component)
		file, err := fp.FilePath(coord)
		if err != nil || file == "" {
			continue
		}
		scope := d.Scope
		if scope == "" {
			scope = ScopeInstance
		}
		out = append(out, SopsPlacement{
			Stack:     s.stack,
			Component: s.component,
			Secret:    d.Name,
			Scope:     scope,
			File:      file,
		})
	}
	return out
}

// DetectSopsCollisions returns an error if the placements violate the scope-vs-file invariants that
// keep stack and instance secrets safe:
//
//   - instance-scoped: two distinct (stack, component) instances must not resolve to the same file
//     (a shared file means an "instance override" would silently overwrite another instance);
//   - stack-scoped: a (stack, secret) must resolve to a single file independent of component (a
//     per-component file means the secret is not actually shared).
//
// The derive-in-code default never trips these; it exists to catch a hand-written `spec.file`
// template that does not discriminate by component (or that discriminates a stack-scoped secret).
func DetectSopsCollisions(placements []SopsPlacement) error {
	defer perf.Track(nil, "secrets.DetectSopsCollisions")()

	instanceOwner := make(map[string]string) // file -> "stack/component"
	stackFile := make(map[string]string)     // stack\x00secret -> file

	for i := range placements {
		p := &placements[i]
		switch p.Scope {
		case ScopeInstance:
			owner := p.Stack + "/" + p.Component
			if prev, ok := instanceOwner[p.File]; ok && prev != owner {
				return fmt.Errorf("%w: instances %q and %q resolve to the same SOPS file %q; add a component discriminator (e.g. {{ .atmos_component }}) to the provider path, or declare the secret stack-scoped",
					ErrSopsCollision, prev, owner, p.File)
			}
			instanceOwner[p.File] = owner
		case ScopeStack:
			key := p.Stack + "\x00" + p.Secret
			if prev, ok := stackFile[key]; ok && prev != p.File {
				return fmt.Errorf("%w: stack-scoped secret %q in stack %q resolves to different files (%q vs %q); it would not be shared across instances",
					ErrSopsCollision, p.Secret, p.Stack, prev, p.File)
			}
			stackFile[key] = p.File
		}
	}
	return nil
}
