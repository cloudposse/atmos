package provider

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateLabelsContext requires a complete review identity before reading its labels.
func ValidateLabelsContext(run *Context) error {
	defer perf.Track(nil, "provider.ValidateLabelsContext")()

	if run == nil || run.PullRequest == nil || run.PullRequest.Number <= 0 || run.RepoOwner == "" || run.RepoName == "" {
		return fmt.Errorf("%w: review repository and number are required", errUtils.ErrInvalidArguments)
	}
	return nil
}
