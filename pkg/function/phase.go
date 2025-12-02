package function

import "github.com/cloudposse/atmos/pkg/perf"

// Phase determines when a function is processed in the stack loading pipeline.
type Phase int

const (
	// PreMerge indicates the function should be processed during initial file loading,
	// before configuration merging. Examples: !env, !exec, !include.
	PreMerge Phase = iota

	// PostMerge indicates the function should be processed after configuration merging,
	// when the full stack context is available. Examples: !terraform.output, !store.get.
	PostMerge
)

// String returns the string representation of the phase.
func (p Phase) String() string {
	defer perf.Track(nil, "function.Phase.String")()

	switch p {
	case PreMerge:
		return "pre-merge"
	case PostMerge:
		return "post-merge"
	default:
		return "unknown"
	}
}
