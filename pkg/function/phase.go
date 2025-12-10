package function

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
// Note: No perf.Track here as this is a trivial accessor called in hot paths.
func (p Phase) String() string {
	switch p {
	case PreMerge:
		return "pre-merge"
	case PostMerge:
		return "post-merge"
	default:
		return "unknown"
	}
}
