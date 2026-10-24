package monorepo

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	gh "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
)

// verifyTimeout bounds the two lookups that verify one module at one ref.
const verifyTimeout = 30 * time.Second

// Verdict is the outcome of comparing the monorepo module with its successor at one ref.
type Verdict string

const (
	// VerdictIdentical means the module and its successor have the same Git tree at the ref.
	VerdictIdentical Verdict = "identical"
	// VerdictDiffers means the ref is missing from a repository, or the trees are not equal.
	VerdictDiffers Verdict = "differs"
	// VerdictUnverified means the comparison could not be completed: no network, rate limit, or no access.
	VerdictUnverified Verdict = "unverified"
)

// Verification is the verdict with the reason a person can act on.
type Verification struct {
	Verdict Verdict
	Reason  string
}

// Verifier proves that a rewritten source selects the same files as the original at one ref.
type Verifier interface {
	Verify(ctx context.Context, result *Result, ref string) Verification
}

// TreeResolver reads the Git tree SHA of a directory at a ref.
type TreeResolver interface {
	TreeSHA(ctx context.Context, owner, repo, ref, dirPath string) (string, error)
}

// githubTrees resolves trees through the shared GitHub client, with its credentials and scoping.
type githubTrees struct{}

func (githubTrees) TreeSHA(ctx context.Context, owner, repo, ref, dirPath string) (string, error) {
	defer perf.Track(nil, "monorepo.githubTrees.TreeSHA")()

	return gh.TreeSHA(ctx, owner, repo, ref, dirPath)
}

// TreeVerifier compares Git tree SHAs: equal trees mean byte-identical directories.
type TreeVerifier struct {
	trees TreeResolver
}

// NewTreeVerifier returns a verifier that reads trees with the given resolver, or from GitHub when
// the resolver is nil.
func NewTreeVerifier(trees TreeResolver) *TreeVerifier {
	defer perf.Track(nil, "monorepo.NewTreeVerifier")()

	if trees == nil {
		trees = githubTrees{}
	}
	return &TreeVerifier{trees: trees}
}

// Verify compares modules/<module> in the monorepo with src in the successor repository at ref.
func (v *TreeVerifier) Verify(ctx context.Context, result *Result, ref string) Verification {
	defer perf.Track(nil, "monorepo.TreeVerifier.Verify")()

	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	legacyPath := ModulesPath + "/" + result.Module
	legacy, err := v.trees.TreeSHA(ctx, LegacyOwner, LegacyRepository, ref, legacyPath)
	if err != nil {
		return classify(err, LegacyOwner+"/"+LegacyRepository, ref)
	}
	successor, err := v.trees.TreeSHA(ctx, SuccessorOwner, result.Repository, ref, SuccessorPath)
	if err != nil {
		return classify(err, result.Slug(), ref)
	}
	if legacy != successor {
		return Verification{VerdictDiffers, fmt.Sprintf("%s/%s at %s (tree %s) differs from %s/%s at %s (tree %s)",
			LegacyRepository, legacyPath, ref, shortSHA(legacy), result.Repository, SuccessorPath, ref, shortSHA(successor))}
	}
	return Verification{VerdictIdentical, fmt.Sprintf("%s/%s and %s/%s are identical at %s (tree %s)",
		LegacyRepository, legacyPath, result.Repository, SuccessorPath, ref, shortSHA(legacy))}
}

// classify turns a lookup failure into a verdict. A missing ref or directory is a definite answer,
// because the successor cannot serve that version. Everything else leaves the question open.
func classify(err error, repository, ref string) Verification {
	switch {
	case errors.Is(err, gh.ErrRefNotFound):
		return Verification{VerdictDiffers, fmt.Sprintf("%s has no tag or ref %q, so the move cannot be verified at that version", repository, ref)}
	case errors.Is(err, gh.ErrPathNotFound):
		return Verification{VerdictDiffers, strings.TrimPrefix(err.Error(), gh.ErrPathNotFound.Error()+": ")}
	case errors.Is(err, errUtils.ErrGitHubRateLimitExceeded):
		return Verification{VerdictUnverified, "GitHub rate limit exceeded; set ATMOS_GITHUB_TOKEN or GITHUB_TOKEN and run again"}
	case errors.Is(err, errUtils.ErrAuthenticationFailed):
		return Verification{VerdictUnverified, "GitHub rejected the credentials; fix or unset ATMOS_GITHUB_TOKEN and GITHUB_TOKEN and run again"}
	}
	return Verification{VerdictUnverified, fmt.Sprintf("could not read %s from GitHub: %v", repository, err)}
}

// shortSHA abbreviates a Git object name for messages.
func shortSHA(sha string) string {
	const length = 12
	if len(sha) > length {
		return sha[:length]
	}
	return sha
}
