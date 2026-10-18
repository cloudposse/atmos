package cli

import (
	"context"
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// HasUnpushedCommits compares local HEAD with its remote tracking branch.
func (p *Provider) HasUnpushedCommits(ctx context.Context, rc atmosgit.RepoContext) (bool, error) {
	defer perf.Track(nil, "cli.Provider.HasUnpushedCommits")()
	// A fresh clone has an upstream but may not have FETCH_HEAD yet.
	ref := "@{upstream}"
	if rc.Branch != "" {
		ref = remoteRef(rc.Remote, rc.Branch)
	}
	result, err := p.runQuiet(ctx, rc.Workdir, rc.Env, "rev-list", "--max-count=1", ref+"..HEAD", "--")
	if err != nil {
		return false, classify(err, result, "inspect unpushed commits")
	}
	return strings.TrimSpace(result.Stdout) != "", nil
}
