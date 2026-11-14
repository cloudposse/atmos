package cli

import (
	"context"
	"fmt"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// checkoutRef pins CI checkouts to the event revision, including when a
// cached worktree is ahead of it. Never merge a moving branch into that SHA.
func (p *Provider) checkoutRef(ctx context.Context, opts *atmosgit.CloneOptions) error {
	args := []string{"fetch"}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprint(opts.Depth))
	}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}
	args = append(args, "--", remoteOrDefault(opts.Remote), opts.Ref)
	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return classify(err, result, "fetch commit")
	}
	result, err = p.run(ctx, opts.Workdir, opts.Env, "checkout", "--detach", "FETCH_HEAD")
	if err != nil {
		return classify(err, result, "checkout commit")
	}
	if opts.Submodules {
		result, err = p.run(ctx, opts.Workdir, opts.Env, "submodule", "update", "--init", "--recursive")
		if err != nil {
			return classify(err, result, "update submodules")
		}
	}
	return nil
}
