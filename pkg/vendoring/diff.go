package vendoring

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// cloneTimeout bounds a clone+diff operation.
const cloneTimeout = 120 * time.Second

// GitDiffer produces a unified diff between two refs of a remote Git repository.
// It is an interface so the diff command can be unit-tested without network.
type GitDiffer interface {
	Diff(ctx context.Context, gitURI, fromRef, toRef, file string) (string, error)
}

// DefaultDiffer is the production GitDiffer backed by go-git.
var DefaultDiffer GitDiffer = &GoGitDiffer{}

// DiffParams configures a vendor diff.
type DiffParams struct {
	// Source is the Git source URL of the component.
	Source string
	// From is the starting ref; defaults to the component's current pinned version.
	From string
	// To is the ending ref; when empty, the latest semver tag is used.
	To string
	// File optionally restricts the diff to a single path.
	File string
	// Lister resolves the latest tag when To is empty; defaults to version.DefaultLister.
	Lister version.RemoteLister
	// Differ performs the clone+diff; defaults to DefaultDiffer.
	Differ GitDiffer
}

// Diff returns a unified diff between two versions of a vendored Git component.
func Diff(atmosConfig *schema.AtmosConfiguration, params *DiffParams) (string, error) {
	defer perf.Track(atmosConfig, "vendoring.Diff")()

	if !version.IsGitSource(params.Source) {
		return "", fmt.Errorf("%w: %s", errUtils.ErrVendorSourceNotGit, params.Source)
	}

	differ := params.Differ
	if differ == nil {
		differ = DefaultDiffer
	}
	lister := params.Lister
	if lister == nil {
		lister = version.DefaultLister
	}

	gitURI := version.ExtractGitURI(params.Source)

	to := params.To
	if to == "" {
		ctx, cancel := context.WithTimeout(context.Background(), listTagsTimeout)
		defer cancel()
		tags, err := lister.ListTags(ctx, gitURI)
		if err != nil {
			return "", err
		}
		if _, latest := version.FindLatestSemVerTag(tags); latest != "" {
			to = latest
		}
	}
	if params.From == "" || to == "" {
		return "", fmt.Errorf("%w: both --from and --to refs are required (could not infer them)", errUtils.ErrVendorDiffFailed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	return differ.Diff(ctx, gitURI, params.From, to, params.File)
}

// GoGitDiffer clones a repository (with tags, no checkout) into a temp directory
// and produces a patch between two commits using go-git — no `git` binary.
type GoGitDiffer struct{}

// Diff clones gitURI and returns the unified diff between fromRef and toRef,
// optionally restricted to a single file path.
func (d *GoGitDiffer) Diff(ctx context.Context, gitURI, fromRef, toRef, file string) (string, error) {
	defer perf.Track(nil, "vendoring.GoGitDiffer.Diff")()

	tmp, err := os.MkdirTemp("", "atmos-vendor-diff-*")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrVendorDiffFailed, err)
	}
	defer os.RemoveAll(tmp)

	repo, err := gogit.PlainCloneContext(ctx, tmp, true, &gogit.CloneOptions{
		URL:        gitURI,
		Tags:       gogit.AllTags,
		NoCheckout: true,
	})
	if err != nil {
		return "", fmt.Errorf("%w: clone %s: %w", errUtils.ErrVendorDiffFailed, gitURI, err)
	}

	fromCommit, err := resolveCommit(repo, fromRef)
	if err != nil {
		return "", err
	}
	toCommit, err := resolveCommit(repo, toRef)
	if err != nil {
		return "", err
	}

	patch, err := fromCommit.Patch(toCommit)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrVendorDiffFailed, err)
	}

	if file != "" {
		return selectFileSections(patch.String(), file), nil
	}
	return patch.String(), nil
}

// resolveCommit resolves a ref (tag, branch, or SHA) to a commit object. It
// tolerates a missing/extra leading "v" so a pinned "2.0.0" resolves a "v2.0.0"
// tag and vice versa.
func resolveCommit(repo *gogit.Repository, ref string) (*object.Commit, error) {
	for _, candidate := range refCandidates(ref) {
		hash, err := repo.ResolveRevision(plumbing.Revision(candidate))
		if err != nil {
			continue
		}
		commit, err := repo.CommitObject(*hash)
		if err != nil {
			continue
		}
		return commit, nil
	}
	return nil, fmt.Errorf("%w: resolve ref %q", errUtils.ErrInvalidGitRef, ref)
}

// refCandidates returns ref plus its v-prefixed/v-stripped variant.
func refCandidates(ref string) []string {
	if strings.HasPrefix(ref, "v") {
		return []string{ref, strings.TrimPrefix(ref, "v")}
	}
	return []string{ref, "v" + ref}
}

// selectFileSections extracts the `diff --git` sections of a unified diff that
// reference the given file path.
func selectFileSections(full, file string) string {
	var out []string
	var keep bool
	for _, line := range strings.Split(full, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			keep = strings.Contains(line, file)
		}
		if keep {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
