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

// wrappedValueFormat wraps a sentinel error with the value it applies to.
const wrappedValueFormat = "%w: %s"

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

	host           = "github.com"
	legacyLocation = host + "/" + LegacyOwner + "/" + LegacyRepository
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
	// ErrSubdirInQuery indicates the subdirectory was written inside the query string, after the ref,
	// which go-getter does not accept. The subdirectory must come before the "?".
	ErrSubdirInQuery = errors.New("subdirectory is inside the query string")
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

	// prefix is the scheme as written, for example "git::https://".
	prefix string
	// host is the host as written, for example "GitHub.com".
	host string
	// dotGit is the .git suffix as written, or empty.
	dotGit string
	query  string
	// querySubdir is a subdirectory misplaced after the ref (ref=1.0//modules/alb), without the
	// leading slashes. It is set only when the source has no subdirectory before the query.
	querySubdir string
	// queryRef is the ref parameter value as written when it carries a misplaced subdirectory.
	queryRef string
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
	// Subpath is the path below the successor's src directory when the source selected a directory
	// inside the module, for example "modules/foo" for modules/eks/cluster/modules/foo. Module is
	// then the mapped parent module, "eks/cluster", which is also what verification compares.
	Subpath string
}

// Slug returns the successor as owner/repository.
func (r *Result) Slug() string {
	defer perf.Track(nil, "monorepo.Result.Slug")()

	return SuccessorOwner + "/" + r.Repository
}

// Mentions reports whether a value names the monorepo in any form, including forms Parse does not
// understand. Doctor uses it to report such values for manual review.
func Mentions(value string) bool {
	defer perf.Track(nil, "monorepo.Mentions")()

	return strings.Contains(strings.ToLower(value), LegacyOwner+"/"+LegacyRepository)
}

// Parse recognizes a go-getter source for the monorepo: github.com/..., https://github.com/...,
// and git::https://github.com/..., with or without .git, a //modules/<module> subdirectory, and
// any query. The scheme, host, owner, and repository match without regard to case, as GitHub
// does. It reports false for every other value.
func Parse(source string) (Reference, bool) {
	defer perf.Track(nil, "monorepo.Parse")()

	prefix, rest := stripSchemes(source)
	location, rest, ok := cutPrefixFold(rest, legacyLocation)
	if !ok {
		return Reference{}, false
	}
	ref := Reference{prefix: prefix, host: location[:len(host)]}
	ref.dotGit, rest, _ = cutPrefixFold(rest, gitSuffix)
	rest, ref.query, _ = strings.Cut(rest, "?")
	switch {
	case rest == "":
	case strings.HasPrefix(rest, subdirMarker):
		ref.Module = moduleOf(strings.TrimPrefix(rest, subdirMarker))
	default:
		return Reference{}, false
	}
	ref.Ref, ref.RefKind = refOf(ref.query)
	if rest == "" {
		ref.splitQuerySubdir()
	}
	return ref, true
}

// stripSchemes removes the optional git:: forcing prefix and https:// scheme, matched without regard
// to case, and returns them as written along with the rest of the source.
func stripSchemes(source string) (prefix, rest string) {
	rest = source
	for _, scheme := range []string{"git::", "https://"} {
		var matched string
		matched, rest, _ = cutPrefixFold(rest, scheme)
		prefix += matched
	}
	return prefix, rest
}

// cutPrefixFold reports whether s starts with prefix without regard to case, and returns the prefix
// as written in s and the remainder. When it does not, matched is empty and rest is s.
func cutPrefixFold(s, prefix string) (matched, rest string, ok bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", s, false
	}
	return s[:len(prefix)], s[len(prefix):], true
}

// splitQuerySubdir separates a subdirectory misplaced after the ref value, as in
// "?ref=1.495.0//modules/alb", from the ref itself.
func (r *Reference) splitQuerySubdir() {
	if r.RefKind != RefLiteral {
		return
	}
	value, subdir, found := strings.Cut(r.Ref, subdirMarker)
	if !found {
		return
	}
	r.queryRef = r.Ref
	r.Ref = value
	r.querySubdir = strings.Trim(subdir, "/")
	if r.Ref == "" {
		r.RefKind = RefNone
	}
}

// QuerySubdirCorrection returns the source with a subdirectory that was written inside the query
// moved in front of it, where go-getter expects it. It is the successor source when the module has
// one, otherwise the corrected monorepo source. It reports false when the subdirectory is not in
// the query.
func (r *Reference) QuerySubdirCorrection() (string, bool) {
	defer perf.Track(nil, "monorepo.Reference.QuerySubdirCorrection")()

	if r.querySubdir == "" {
		return "", false
	}
	query := strings.Replace(r.query, "ref="+r.queryRef, "ref="+r.Ref, 1)
	if r.Ref == "" {
		query = strings.Replace(r.query, "ref="+r.queryRef, "", 1)
		query = strings.Trim(strings.ReplaceAll(query, "&&", "&"), "&")
	}
	corrected := r.prefix + r.host + "/" + LegacyOwner + "/" + LegacyRepository + r.dotGit + subdirMarker + r.querySubdir
	if query != "" {
		corrected += "?" + query
	}
	if reference, ok := Parse(corrected); ok {
		if result, err := reference.Rewrite(); err == nil {
			return result.Source, true
		}
	}
	return corrected, true
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
// ref, the host spelling, and every other query parameter are kept as written. A directory inside a
// mapped module, such as modules/eks/cluster/modules/foo, moves to the same directory below src. It
// fails with ErrSubdirInQuery, ErrNoModule, ErrTemplatedModule, ErrSplitComponent, or
// ErrNoSuccessor, wrapped with the module name.
func (r *Reference) Rewrite() (Result, error) {
	defer perf.Track(nil, "monorepo.Reference.Rewrite")()

	switch {
	case r.querySubdir != "":
		return Result{}, fmt.Errorf(wrappedValueFormat, ErrSubdirInQuery, r.querySubdir)
	case r.Module == "":
		return Result{}, ErrNoModule
	case strings.Contains(r.Module, templateOpen):
		return Result{}, fmt.Errorf(wrappedValueFormat, ErrTemplatedModule, r.Module)
	}
	module, subpath := r.Module, ""
	repository, ok := Repository(module)
	if !ok {
		module, subpath, ok = ParentModule(r.Module)
		if ok {
			repository, ok = Repository(module)
		}
	}
	if !ok {
		if len(Successors(r.Module)) > 0 {
			return Result{}, fmt.Errorf(wrappedValueFormat, ErrSplitComponent, r.Module)
		}
		return Result{}, fmt.Errorf(wrappedValueFormat, ErrNoSuccessor, r.Module)
	}
	source := r.prefix + r.host + "/" + SuccessorOwner + "/" + repository + r.dotGit + subdirMarker + SuccessorPath
	if subpath != "" {
		source += "/" + subpath
	}
	if r.query != "" {
		source += "?" + r.query
	}
	return Result{Source: source, Module: module, Subpath: subpath, Repository: repository, Ref: r.Ref, RefKind: r.RefKind}, nil
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
