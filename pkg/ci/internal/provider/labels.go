package provider

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateLabelsContext requires a complete review identity before reading its labels.
func ValidateLabelsContext(run *Context) error {
	defer perf.Track(nil, "provider.ValidateLabelsContext")()

	if run == nil || LabelsReviewNumber(run) <= 0 || run.RepoOwner == "" || run.RepoName == "" {
		if run != nil && run.MergeQueue != nil {
			return fmt.Errorf("%w: merge-queue review repository and number are required (queue ref %q)", errUtils.ErrInvalidArguments, run.MergeQueue.HeadRef)
		}
		return fmt.Errorf("%w: review repository and number are required", errUtils.ErrInvalidArguments)
	}
	return nil
}

// LabelsReviewNumber identifies the review supplying labels for this run.
func LabelsReviewNumber(run *Context) int {
	defer perf.Track(nil, "provider.LabelsReviewNumber")()

	if run == nil {
		return 0
	}
	if run.MergeQueue != nil {
		return run.MergeQueue.PullRequestNumber
	}
	if run.PullRequest != nil {
		return run.PullRequest.Number
	}
	return 0
}
