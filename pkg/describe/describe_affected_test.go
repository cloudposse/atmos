package describe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/tests"
)

// describeAffectedCloneRetryBudget bounds retries of the real GitHub clone this test performs.
// A transient DNS/TLS blip reaching github.com occasionally fails the clone even moments after
// RequireGitHubAccess confirmed reachability (and CI sets ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true,
// making that check a no-op there anyway — see docs/fixes/2026-09-08-terraform-plugin-cache-windows-registry-flake.md
// for the equivalent registry.terraform.io incident this mirrors). A real failure (bad ref, auth)
// fails identically on every attempt and still fails the test once the budget is spent.
const describeAffectedCloneRetryBudget = 30 * time.Second

func TestDescribeAffectedWithTargetRefClone(t *testing.T) {
	// Skip long tests in short mode (this test takes ~36 seconds due to Git cloning)
	tests.SkipIfShort(t)

	// Check for Git repository with valid remotes and GitHub access (for cloning)
	tests.RequireGitRemoteWithValidURL(t)
	tests.RequireGitHubAccess(t)
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	// Git reference and commit SHA
	// Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details
	ref := "refs/heads/main"
	sha := ""

	var affected []schema.Affected
	deadline := time.Now().Add(describeAffectedCloneRetryBudget)
	for time.Now().Before(deadline) {
		affected, _, _, _, err = e.ExecuteDescribeAffectedWithTargetRefClone(
			&atmosConfig,
			ref,
			sha,
			"",
			"",
			true,
			true,
			"",
			true,
			true,
			nil,
			false,
			nil,   // authManager
			false, // authDisabled
		)
		if err == nil {
			break
		}
		t.Logf("ExecuteDescribeAffectedWithTargetRefClone failed, retrying: %v", err)
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		sleep := 500 * time.Millisecond
		if remaining < sleep {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if affectedYaml != "" {
				t.Logf("Affected components and stacks:\n%s", affectedYaml)
			} else {
				t.Logf("Affected components and stacks (raw): %+v", affected)
			}
		}
	})
}

func TestDescribeAffectedWithTargetRepoPath(t *testing.T) {
	// Check for Git repository with valid remotes precondition
	tests.RequireGitRemoteWithValidURL(t)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	// Point to the same local repository
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list
	repoPath := "../../"

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		true,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,   // authManager
		false, // authDisabled
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if affectedYaml != "" {
				t.Logf("Affected components and stacks:\n%s", affectedYaml)
			} else {
				t.Logf("Affected components and stacks (raw): %+v", affected)
			}
		}
	})
}
