package github

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
)

// PolicySnapshotSchema identifies the local GitHub Actions snapshot contract.
const PolicySnapshotSchema = "github-actions/v1"

// CollectPolicySnapshot captures native Actions metadata and its supplied event.
// This requires no API permissions or credentials. Event data is triggering
// evidence, NOT a fresh review/PR lookup. Credentials and arbitrary env are never
// copied, and no shell, Git command or Atmos configuration is executed.
func (p *Provider) CollectPolicySnapshot(ctx context.Context) (*policy.CISnapshot, error) {
	defer perf.Track(nil, "github.Provider.CollectPolicySnapshot")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	metadata := map[string]string{}
	for _, key := range []string{
		"GITHUB_ACTIONS", "GITHUB_EVENT_NAME", "GITHUB_REPOSITORY", "GITHUB_REPOSITORY_ID",
		"GITHUB_REPOSITORY_OWNER", "GITHUB_SHA", "GITHUB_REF", "GITHUB_REF_NAME", "GITHUB_REF_TYPE",
		"GITHUB_BASE_REF", "GITHUB_HEAD_REF", "GITHUB_ACTOR", "GITHUB_ACTOR_ID", "GITHUB_TRIGGERING_ACTOR",
		"GITHUB_RUN_ID", "GITHUB_RUN_NUMBER", "GITHUB_RUN_ATTEMPT", "GITHUB_WORKFLOW", "GITHUB_JOB",
		"GITHUB_SERVER_URL", "GITHUB_API_URL", "RUNNER_OS", "RUNNER_ARCH", "RUNNER_ENVIRONMENT",
	} {
		if value, exists := os.LookupEnv(key); exists {
			metadata[key] = value
		}
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	provenance := policy.Provenance{Source: "github-actions:environment", CollectedAt: time.Now().UTC(), Revision: metadata["GITHUB_SHA"]}
	snapshot := &policy.CISnapshot{
		Provider: p.Name(), Schema: PolicySnapshotSchema,
		Metadata: policy.Dataset{Data: data, Status: policy.Available, Complete: true, Provenance: provenance},
		Facts:    map[string]policy.Dataset{},
	}
	snapshot.Event = collectPolicyEvent(provenance)
	return snapshot, ctx.Err()
}

func collectPolicyEvent(provenance policy.Provenance) policy.Dataset {
	provenance.Source = "github-actions:GITHUB_EVENT_PATH"
	dataset := policy.Dataset{Status: policy.Unavailable, Provenance: provenance, Reason: "native event file is unavailable"}
	path, _ := os.LookupEnv("GITHUB_EVENT_PATH")
	if path == "" {
		return dataset
	}
	// The trusted runner supplies this absolute provider-native path, not a PR payload URL/path.
	file, err := os.Open(path)
	if err != nil {
		return dataset
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return dataset
	}
	const limit = 8 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	var object map[string]json.RawMessage
	if err != nil || len(data) > limit || json.Unmarshal(data, &object) != nil || object == nil {
		dataset.Status = policy.Incomplete
		dataset.Reason = "native event is invalid or exceeds the collection limit"
		return dataset
	}
	dataset.Data, dataset.Status, dataset.Complete, dataset.Reason = data, policy.Available, true, ""
	return dataset
}
