// Package monorepo moves component sources from the archived cloudposse/terraform-aws-components
// monorepo to the per-component repositories in the cloudposse-terraform-components organization.
//
// The move keeps the version: the successor repositories carry the monorepo's historical tags
// unchanged, and the contents of src/ equal the contents of modules/<module> at the same tag. This
// package rewrites the source string and proves the equality for one tag; it never changes a version.
package monorepo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// LegacyOwner and LegacyRepository identify the archived monorepo.
	LegacyOwner      = "cloudposse"
	LegacyRepository = "terraform-aws-components"
	// SuccessorOwner is the GitHub organization that holds the per-component repositories.
	SuccessorOwner = "cloudposse-terraform-components"
	// SuccessorPath is the directory of a successor repository that holds the component code.
	SuccessorPath = "src"
	// ModulesPath is the monorepo directory that holds the component modules.
	ModulesPath = "modules"
	// MigrationGuideURL documents the migration, including the components that were split.
	MigrationGuideURL = "https://docs.cloudposse.com/learn/maintenance/tutorials/how-to-update-components-yaml-to-new-organization/"

	legacyLocation = "github.com/" + LegacyOwner + "/" + LegacyRepository
	gitSuffix      = ".git"
	subdirMarker   = "//"
	templateOpen   = "{{"
)

var (
	// ErrNoModule indicates the source does not select a module below modules/.
	ErrNoModule = errors.New("source does not select a module of the monorepo")
	// ErrTemplatedModule indicates the module path is a template, so the module cannot be determined.
	ErrTemplatedModule = errors.New("module path is a template")
	// ErrSplitComponent indicates the module was split into several repositories, one per subcomponent.
	ErrSplitComponent = errors.New("component was split into several repositories")
	// ErrNoSuccessor indicates the module has no successor repository.
	ErrNoSuccessor = errors.New("component has no successor repository")
)

// versionTemplate matches the ref placeholder that vendoring fills from the version field.
var versionTemplate = regexp.MustCompile(`^\{\{\s*\.Version\s*\}\}$`)

// RefKind says how a source pins the monorepo revision.
type RefKind int

const (
	// RefNone means the source has no ref query parameter.
	RefNone RefKind = iota
	// RefLiteral means the ref is written out, for example ref=1.495.0.
	RefLiteral
	// RefVersion means the ref is the {{.Version}} placeholder, resolved from the version field.
	RefVersion
	// RefDynamic means the ref is any other template and cannot be resolved statically.
	RefDynamic
)

// Reference is a source string that points at the archived monorepo.
type Reference struct {
	// Module is the module path below modules/, for example "eks/cluster". It is empty when the
	// source selects no module.
	Module string
	// Ref is the revision as written: a literal ref, or the placeholder text for RefVersion.
	Ref     string
	RefKind RefKind

	prefix string
	dotGit bool
	query  string
}

// Result is a source rewritten to the successor repository.
type Result struct {
	// Source is the rewritten source string.
	Source string
	// Module is the monorepo module the source selected.
	Module string
	// Repository is the successor repository name, for example "aws-identity-center".
	Repository string
	// Ref is the literal revision, or the placeholder text when RefKind is RefVersion.
	Ref     string
	RefKind RefKind
}

// Slug returns the successor as owner/repository.
func (r Result) Slug() string {
	defer perf.Track(nil, "monorepo.Result.Slug")()

	return SuccessorOwner + "/" + r.Repository
}

// Mentions reports whether a value names the monorepo in any form, including forms Parse does not
// understand. Doctor uses it to report such values for manual review.
func Mentions(value string) bool {
	defer perf.Track(nil, "monorepo.Mentions")()

	return strings.Contains(value, LegacyOwner+"/"+LegacyRepository)
}

// Parse recognizes a go-getter source for the monorepo: github.com/..., https://github.com/...,
// and git::https://github.com/..., with or without .git, a //modules/<module> subdirectory, and
// any query. It reports false for every other value.
func Parse(source string) (Reference, bool) {
	defer perf.Track(nil, "monorepo.Parse")()

	rest := source
	prefix := ""
	for _, scheme := range []string{"git::", "https://"} {
		if strings.HasPrefix(rest, scheme) {
			prefix += scheme
			rest = strings.TrimPrefix(rest, scheme)
		}
	}
	if !strings.HasPrefix(rest, legacyLocation) {
		return Reference{}, false
	}
	rest = strings.TrimPrefix(rest, legacyLocation)
	ref := Reference{prefix: prefix}
	if strings.HasPrefix(rest, gitSuffix) {
		ref.dotGit = true
		rest = strings.TrimPrefix(rest, gitSuffix)
	}
	rest, ref.query, _ = strings.Cut(rest, "?")
	switch {
	case rest == "":
	case strings.HasPrefix(rest, subdirMarker):
		ref.Module = moduleOf(strings.TrimPrefix(rest, subdirMarker))
	default:
		return Reference{}, false
	}
	ref.Ref, ref.RefKind = refOf(ref.query)
	return ref, true
}

// moduleOf extracts the module path from a subdirectory such as modules/eks/cluster.
func moduleOf(subdir string) string {
	module, ok := strings.CutPrefix(subdir, ModulesPath+"/")
	if !ok {
		return ""
	}
	return strings.TrimSuffix(module, "/")
}

// refOf finds the ref parameter in a raw query and classifies it.
func refOf(query string) (string, RefKind) {
	for _, parameter := range strings.Split(query, "&") {
		value, ok := strings.CutPrefix(parameter, "ref=")
		if !ok {
			continue
		}
		switch {
		case value == "":
			return "", RefNone
		case versionTemplate.MatchString(value):
			return value, RefVersion
		case strings.Contains(value, templateOpen):
			return value, RefDynamic
		}
		return value, RefLiteral
	}
	return "", RefNone
}

// Rewrite moves the reference to its successor repository. The scheme prefix, the .git suffix, the
// ref, and every other query parameter are kept as written. It fails with ErrNoModule,
// ErrTemplatedModule, ErrSplitComponent, or ErrNoSuccessor, wrapped with the module name.
func (r *Reference) Rewrite() (Result, error) {
	defer perf.Track(nil, "monorepo.Reference.Rewrite")()

	switch {
	case r.Module == "":
		return Result{}, ErrNoModule
	case strings.Contains(r.Module, templateOpen):
		return Result{}, fmt.Errorf("%w: %s", ErrTemplatedModule, r.Module)
	}
	repository, ok := Repository(r.Module)
	if !ok {
		if len(Successors(r.Module)) > 0 {
			return Result{}, fmt.Errorf("%w: %s", ErrSplitComponent, r.Module)
		}
		return Result{}, fmt.Errorf("%w: %s", ErrNoSuccessor, r.Module)
	}
	source := r.prefix + "github.com/" + SuccessorOwner + "/" + repository
	if r.dotGit {
		source += gitSuffix
	}
	source += subdirMarker + SuccessorPath
	if r.query != "" {
		source += "?" + r.query
	}
	return Result{Source: source, Module: r.Module, Repository: repository, Ref: r.Ref, RefKind: r.RefKind}, nil
}

// Rewrite parses a source and moves it to its successor repository. It reports false when the
// source does not name the monorepo or its module has no automatic successor.
func Rewrite(source string) (Result, bool) {
	defer perf.Track(nil, "monorepo.Rewrite")()

	reference, ok := Parse(source)
	if !ok {
		return Result{}, false
	}
	result, err := reference.Rewrite()
	return result, err == nil
}
