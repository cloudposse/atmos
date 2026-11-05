package secrets

import (
	"errors"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// rootFolder is the widest conservative location: everything under the working directory. It is
// used when nothing is known about where a provider's files live.
const rootFolder = "."

// FileDependencySet is the result of ResolveFileDependencies: the files a scope's file-based
// secrets are known to live in, plus the conservative locations for declarations whose backend
// selector could not be resolved.
type FileDependencySet struct {
	// Files are the exact backing files of declarations that resolved.
	Files []string
	// Folders are directories under which a change may affect the scope, because a declaration's
	// backend selector could not be resolved and its file is therefore unknown.
	Folders []string
	// Unresolved lists each declaration that contributed to Folders, so callers can report it.
	Unresolved []UnresolvedFileDependency
}

// UnresolvedFileDependency describes one SOPS declaration whose backend selector could not be
// resolved, and the conservative locations that stand in for its unknown file.
type UnresolvedFileDependency struct {
	// Declaration is the secret name.
	Declaration string
	// Field is where the selector sits: `sops` for the declaration's own backend, or
	// `providers.<name>` for a selector inside the named provider definition.
	Field string
	// Selector is the unresolved selector text.
	Selector string
	// Locations are the files and folders that stand in for the unknown file.
	Locations providers.FileLocations
	// Err is why the selector was not resolved.
	Err error
}

// FileDependencies returns the distinct backing files this scope's file-based declared secrets
// (currently SOPS) resolve to. It is the exact-file part of ResolveFileDependencies; callers that
// must not under-report changes (`describe affected`) use ResolveFileDependencies, which also
// returns the conservative folders for selector-backed declarations that could not be resolved.
// Results are de-duplicated and sorted.
func (s *Service) FileDependencies() []string {
	defer perf.Track(s.atmosConfig, "secrets.Service.FileDependencies")()

	return s.ResolveFileDependencies().Files
}

// ResolveFileDependencies returns the files this scope's file-based declared secrets depend on, for
// `describe affected` to treat as implicit dependencies: a changed secret file marks every
// component that consumes it. Secrets whose backend is not file-based (store-backed) contribute
// nothing, and other declarations that cannot be resolved are skipped.
//
// A SOPS declaration whose backend selector cannot be resolved (no evaluator, which is the case for
// credential-free `describe affected`, or an undeployed producer) is never skipped silently. Its
// file is unknown, so the possible locations of the provider definitions it could name are
// returned as Folders (and Files) and the declaration is listed in Unresolved: any change there
// may affect the scope. When the service has a selector evaluator and the selector resolves, the
// exact file is returned instead. Results are de-duplicated and sorted.
func (s *Service) ResolveFileDependencies() FileDependencySet {
	defer perf.Track(s.atmosConfig, "secrets.Service.ResolveFileDependencies")()

	files := make(map[string]bool)
	folders := make(map[string]bool)
	var set FileDependencySet
	for _, decl := range s.Declarations() {
		d := decl
		provider, err := s.provider(&d)
		if err != nil {
			if d.BackendType == BackendSops && errors.Is(err, ErrSelectorUnresolved) {
				u := s.unresolvedFileDependency(&d, err)
				set.Unresolved = append(set.Unresolved, u)
				addAll(files, u.Locations.Files)
				addAll(folders, u.Locations.Folders)
			}
			continue
		}
		fp, ok := provider.(providers.FilePathProvider)
		if !ok {
			continue
		}
		path, err := fp.FilePath(coordinateForDeclaration(&d, s.stack, s.component))
		if err != nil || path == "" {
			continue
		}
		files[path] = true
	}
	set.Files = sortedKeys(files)
	set.Folders = sortedKeys(folders)
	return set
}

// unresolvedFileDependency builds the conservative stand-in for a SOPS declaration whose backend
// selector could not be resolved. A selector in the declaration's `sops:` could name any SOPS
// provider, so every known provider definition contributes; a selector inside a named provider
// definition only widens that provider.
func (s *Service) unresolvedFileDependency(decl *Declaration, cause error) UnresolvedFileDependency {
	u := UnresolvedFileDependency{Declaration: decl.Name, Err: cause}
	definitions := s.sopsProviderDefinitions()

	var candidates []any
	if IsSelector(decl.BackendName) {
		u.Field = string(BackendSops)
		u.Selector = decl.BackendName
		for _, def := range definitions {
			candidates = append(candidates, def)
		}
	} else {
		u.Field = providersSectionKey + "." + decl.BackendName
		u.Selector = selectorText(definitions[decl.BackendName])
		candidates = append(candidates, definitions[decl.BackendName])
	}
	if len(candidates) == 0 {
		// No provider definition is known, so only the provider default applies.
		candidates = append(candidates, map[string]any(nil))
	}

	files := make(map[string]bool)
	folders := make(map[string]bool)
	for _, def := range candidates {
		loc := definitionLocations(def)
		addAll(files, loc.Files)
		addAll(folders, loc.Folders)
	}
	u.Locations = providers.FileLocations{Files: sortedKeys(files), Folders: sortedKeys(folders)}
	return u
}

// sopsProviderDefinitions returns the raw SOPS provider definitions visible to this scope: those
// declared in the component's `secrets.providers`, which win, and those in atmos.yaml.
func (s *Service) sopsProviderDefinitions() map[string]any {
	out := make(map[string]any)
	if s.atmosConfig != nil {
		for name, def := range s.atmosConfig.Secrets.Providers {
			out[name] = map[string]any{"kind": def.Kind, "spec": def.Spec}
		}
	}
	for name, def := range ExtractProviders(s.componentSection) {
		out[name] = def
	}
	return out
}

// definitionLocations returns the possible file locations of one raw provider definition. A
// definition (or its `spec`) that is itself a selector is unknown and covers the whole working
// directory.
func definitionLocations(def any) providers.FileLocations {
	var spec map[string]any
	switch d := def.(type) {
	case nil:
	case map[string]any:
		switch raw := d["spec"].(type) {
		case nil:
		case map[string]any:
			spec = raw
		default:
			return providers.FileLocations{Folders: []string{rootFolder}}
		}
	default:
		return providers.FileLocations{Folders: []string{rootFolder}}
	}
	loc, ok := providers.PossibleLocations(providers.TrackSops, spec)
	if !ok {
		return providers.FileLocations{Folders: []string{rootFolder}}
	}
	return loc
}

func addAll(set map[string]bool, values []string) {
	for _, v := range values {
		set[v] = true
	}
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
