package github

import (
	"os"
	"regexp"
	"strconv"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// Match the final PR segment, including when the target branch contains slashes.
// The suffix is a queue-generated SHA, not necessarily the PR's head SHA.
var mergeQueueRef = regexp.MustCompile(`^refs/heads/gh-readonly-queue/.+/pr-([1-9][0-9]*)-[[:xdigit:]]+$`)

// mergeQueueFromEvent identifies the PR for this queue run without any API calls.
// Keep unresolved queue identity visible so label evaluation reports an error
// rather than confusing an unsupported ref with a run that has no review.
func mergeQueueFromEvent() *provider.MergeQueueInfo {
	ref := ""
	if payload, err := readEventPayload(); err == nil {
		group, _ := payload[payloadKeyMergeGroup].(map[string]any)
		ref, _ = group["head_ref"].(string)
	}
	if ref == "" {
		ref = os.Getenv("GITHUB_REF")
	}
	if ref == "" {
		if name := os.Getenv("GITHUB_REF_NAME"); name != "" {
			ref = refsHeadsPrefix + name
		}
	}
	info := &provider.MergeQueueInfo{HeadRef: ref}
	if match := mergeQueueRef.FindStringSubmatch(ref); match != nil {
		number, err := strconv.Atoi(match[1])
		if err == nil {
			info.PullRequestNumber = number
		}
	}
	return info
}
