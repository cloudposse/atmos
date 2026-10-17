package ci

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
)

// PolicySnapshotProvider is an optional Native CI provider capability. It leaves
// Context and the existing Provider interface unchanged. Payload schemas belong
// to the provider; collectors must not execute Atmos configuration.
type PolicySnapshotProvider interface {
	CollectPolicySnapshot(context.Context) (*policy.CISnapshot, error)
}

// CollectPolicySnapshot uses the existing Native CI registry. No detected provider
// means no CI context, not fabricated local CI evidence. Required inputs are
// checked by the policy engine before predicates run.
func CollectPolicySnapshot(ctx context.Context) (*policy.CISnapshot, error) {
	defer perf.Track(nil, "ci.CollectPolicySnapshot")()
	p := Detect()
	if p == nil {
		return nil, nil
	}
	if collector, ok := p.(PolicySnapshotProvider); ok {
		return collector.CollectPolicySnapshot(ctx)
	}
	return &policy.CISnapshot{
		Provider: p.Name(),
		Metadata: policy.Dataset{Status: policy.Unavailable, Reason: "provider has no policy snapshot capability"},
		Event:    policy.Dataset{Status: policy.Unavailable, Reason: "provider has no native event capability"},
	}, nil
}
