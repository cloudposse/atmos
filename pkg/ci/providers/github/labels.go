package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

var _ provider.LabelReader = (*Provider)(nil)

const labelsPerPage = 100

// Labels reads all current pull request labels through the GitHub Issues API.
func (p *Provider) Labels(ctx context.Context, run *provider.Context) ([]string, error) {
	defer perf.Track(nil, "github.Provider.Labels")()

	if err := provider.ValidateLabelsContext(run); err != nil {
		return nil, err
	}
	if err := p.ensureClient(); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCILabelsFetchFailed, err)
	}
	options := &github.ListOptions{PerPage: labelsPerPage}
	labels := []string{}
	for {
		page, response, err := p.client.GitHub().Issues.ListLabelsByIssue(ctx, run.RepoOwner, run.RepoName, run.PullRequest.Number, options)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrCILabelsFetchFailed, err)
		}
		for _, label := range page {
			labels = append(labels, label.GetName())
		}
		if response.NextPage == 0 {
			return labels, nil
		}
		options.Page = response.NextPage
	}
}
