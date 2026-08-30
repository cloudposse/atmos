package github

import (
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// legacyActionPrefix identifies the older cloudposse/github-action-atmos-*
// marketplace actions that predate Native CI.
const legacyActionPrefix = "cloudposse/github-action-atmos-"

// LegacyActionRepo reports the owner/repo of the currently executing GitHub
// Action when it is one of the older cloudposse/github-action-atmos-*
// marketplace actions. GitHub Actions sets GITHUB_ACTION_REPOSITORY for the
// step currently running, including a composite action's own nested run:
// steps, so this also detects atmos invoked from inside such an action.
func LegacyActionRepo() (repo string, ok bool) {
	defer perf.Track(nil, "github.LegacyActionRepo")()

	repo = os.Getenv("GITHUB_ACTION_REPOSITORY")
	return repo, strings.HasPrefix(repo, legacyActionPrefix)
}
