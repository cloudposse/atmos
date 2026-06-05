package artifact

// Query defines filter criteria for listing artifacts.
type Query struct {
	// Components filters by component names.
	Components []string

	// Stacks filters by stack names.
	Stacks []string

	// SHAs filters by git commit SHAs.
	SHAs []string

	// All returns all artifacts regardless of other filters.
	All bool
}
