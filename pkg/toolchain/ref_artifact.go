package toolchain

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ResolveRef resolves a git ref (branch or tag name) for the Atmos repository to
// its full commit SHA. The returned SHA can be passed to InstallFromSHA to install
// the build artifact for the ref's latest commit.
//
// Because the resolution happens on every invocation, mutable refs (e.g. "main")
// are handled naturally: the SHA-keyed install cache is a hit when the ref has not
// moved and a miss (triggering install) when it has.
func ResolveRef(ctx context.Context, ref string) (string, error) {
	defer perf.Track(nil, "toolchain.ResolveRef")()

	log.Debug("Resolving git ref to SHA", "ref", ref)

	sha, err := github.GetRefSHA(ctx, atmosOwner, atmosRepo, ref)
	if err != nil {
		return "", handleRefResolveError(err, ref)
	}

	return sha, nil
}

// handleRefResolveError converts ref-resolution errors into user-friendly errors.
func handleRefResolveError(err error, ref string) error {
	refURL := fmt.Sprintf("https://github.com/%s/%s/tree/%s", atmosOwner, atmosRepo, ref)

	if errors.Is(err, github.ErrRefNotFound) {
		return errUtils.Build(errUtils.ErrToolNotFound).
			WithExplanationf("Git ref '%s' not found in %s/%s", ref, atmosOwner, atmosRepo).
			WithHint("Check the branch or tag name (e.g. ref:main, ref:v1.199.0)").
			WithHint("Disambiguate a branch vs tag with ref:heads/<name> or ref:tags/<name>").
			WithHintf("Check the ref: %s", refURL).
			WithExitCode(1).
			Err()
	}

	return errUtils.Build(errUtils.ErrToolInstall).
		WithCause(err).
		WithExplanationf("Failed to resolve git ref '%s'", ref).
		WithHintf("Check the ref: %s", refURL).
		WithExitCode(1).
		Err()
}
