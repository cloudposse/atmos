package function

// Phase represents when a function should be executed during configuration processing.
type Phase int

const (
	// PreMerge functions are executed during initial file loading, before
	// configuration merging. Examples: !env, !exec, !include, !random.
	PreMerge Phase = iota

	// PostMerge functions are executed after configuration merging, when the
	// full stack context is available. Examples: !terraform.output, !store.get.
	PostMerge
)

// String returns a human-readable representation of the phase.
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
