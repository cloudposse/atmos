package report_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
)

// TestRenderGoldens renders one summary per outcome and compares it with the golden file.
func TestRenderGoldens(t *testing.T) {
	copied := result(&artifact.OperationResult{
		OperationID: "c", Action: artifact.ActionCopy, Outcome: artifact.OutcomeCreated, Repository: "mirror",
		Requested: "components/vpc:1.2.0", Ref: "components/vpc:1.2.0", Location: "s3://backups/components/vpc/1.2.0",
		ResolvedDigest: digestOf("a"), Size: 1 << 20, Duration: 2500 * time.Millisecond, LockScope: artifact.LockScopeRepository,
	})
	pulled := result(&artifact.OperationResult{
		OperationID: "p", Action: artifact.ActionPull, Outcome: artifact.OutcomeRead, Repository: "shared",
		Requested: "components/vpc:latest", Ref: "components/vpc:1.2.0", Location: "/work/components/vpc",
		ContentDigest: digestOf("c"), RepresentationDigest: digestOf("d"), Size: 49152,
		Aliases: []string{"components/vpc:latest"},
	})
	deleted := result(&artifact.OperationResult{
		OperationID: "d", Action: artifact.ActionDelete, Outcome: artifact.OutcomeDeleted, Repository: "shared",
		Requested: "components/vpc:0.9.0", Ref: "components/vpc:0.9.0", LockScope: artifact.LockScopeHost,
	})
	unchanged := published("u", "components/vpc:1.2.0")
	unchanged.Result.Outcome = artifact.OutcomeUnchanged
	replaced := published("r", "components/vpc:1.2.0")
	replaced.Result.Outcome = artifact.OutcomeReplaced
	dryRun := published("y", "components/vpc:1.2.0")
	dryRun.Result.Outcome = artifact.OutcomeDryRun
	dryRun.Result.LockScope = ""
	listed := &invoke.Outcome{Collection: &invoke.Collection{
		Artifacts: []invoke.Listed{{Ref: "components/vpc:1.2.0"}, {Ref: "components/vpc:1.1.0"}},
		Repositories: []artifact.RepositoryListing{
			{Repository: "shared", Count: 2, Source: "live"},
			{Repository: "mirror", Count: 0, Source: "cache"},
		},
		Incomplete: true,
	}}
	failedPublish := result(&artifact.OperationResult{OperationID: "f", Action: artifact.ActionPublish, Outcome: artifact.OutcomeFailed, Repository: "shared", Requested: "components/vpc:1.2.0"})

	cases := map[string][]*report.Entry{
		"published":   {entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil)},
		"unchanged":   {entry("publish", "u", 1, unchanged, nil)},
		"replaced":    {entry("publish", "r", 1, replaced, nil)},
		"dry-run":     {entry("publish", "y", 1, dryRun, nil)},
		"downloaded":  {entry("pull", "p", 1, pulled, nil)},
		"copied":      {entry("copy", "c", 1, copied, nil)},
		"deleted":     {entry("delete", "d", 1, deleted, nil)},
		"signed":      {entry("sign", "s", 1, signed("s"), nil)},
		"verified":    {entry("verify", "v", 1, verified("v", artifact.VerificationVerified), nil)},
		"unchecked":   {entry("verify", "v", 1, verified("v", artifact.VerificationUnchecked), nil)},
		"failed":      {entry("publish", "f", 3, failedPublish, fmt.Errorf("%w: conflict on components/vpc:1.2.0", errUtils.ErrArtifactConflict))},
		"canceled":    {entry("publish", "f", 1, failedPublish, fmt.Errorf("publish: %w", context.Canceled))},
		"timed-out":   {entry("publish", "f", 1, failedPublish, fmt.Errorf("publish: %w", context.DeadlineExceeded))},
		"listed":      {{Step: "list", Action: artifact.ActionList, Attempt: 1, Outcome: listed}},
		"verify-fail": {entry("verify", "v", 1, verified("v", artifact.VerificationFailed), fmt.Errorf("%w: wrong issuer", errUtils.ErrArtifactVerificationFailed))},
		"mixed": {
			entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil, 0),
			entry("sign", "s", 1, signed("s"), nil, 2),
			entry("verify", "v", 1, verified("v", artifact.VerificationVerified), nil, 4),
			entry("copy", "c", 1, copied, nil, 6),
			entry("publish", "f", 1, failedPublish, errors.New("registry unavailable"), 8),
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			golden(t, name, flushOne(t, enabledConfig(), entries...))
		})
	}
}

// TestRenderStatusWording asserts the outcome wording and kind without depending on the goldens.
func TestRenderStatusWording(t *testing.T) {
	cases := []struct {
		name    string
		entries []*report.Entry
		want    []string
	}{
		{"published", []*report.Entry{entry("s", "a", 1, published("a", "x/y:1"), nil)}, []string{"✅ Published", "1 succeeded"}},
		{"unchanged counts once", []*report.Entry{entry("s", "a", 1, result(&artifact.OperationResult{Action: artifact.ActionPublish, Outcome: artifact.OutcomeUnchanged}), nil)}, []string{"➖ Unchanged", "1 unchanged"}},
		{"canceled sibling", []*report.Entry{entry("s", "a", 1, result(&artifact.OperationResult{Action: artifact.ActionPull}), context.Canceled)}, []string{"🚫 Canceled", "1 canceled"}},
		{"failed", []*report.Entry{entry("s", "a", 1, result(&artifact.OperationResult{Action: artifact.ActionCopy}), errors.New("nope"))}, []string{"❌ Failed", "1 failed"}},
		{"unchecked is not verified", []*report.Entry{entry("s", "a", 1, verified("a", artifact.VerificationUnchecked), nil)}, []string{"⚠️ Unchecked", "nothing is claimed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			markdown := flushOne(t, enabledConfig(), tc.entries...)
			for _, want := range tc.want {
				assert.Contains(t, markdown, want)
			}
		})
	}
	// Unchecked and signed must never read as verified.
	markdown := flushOne(t, enabledConfig(), entry("s", "a", 1, verified("a", artifact.VerificationUnchecked), nil))
	assert.NotContains(t, markdown, "publisher is authenticated")
	markdown = flushOne(t, enabledConfig(), entry("s", "a", 1, signed("a"), nil))
	assert.Contains(t, markdown, "Signing alone does not authenticate the signer")
	assert.NotContains(t, markdown, "publisher is authenticated")
}
