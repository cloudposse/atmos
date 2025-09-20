package ci

// CommentMetadata contains common metadata for CI comments.
type CommentMetadata struct {
	UUID      string
	Provider  string
	JobID     string
	RunNumber int
}

// CommentOptions contains options for posting comments.
type CommentOptions struct {
	Strategy      string // always, never, adaptive, on-failure, on-skip, platform-specific
	Discriminator string // Job discriminator for multi-job scenarios
}

// SummaryOptions contains options for job summaries.
type SummaryOptions struct {
	OutputFile string
	Format     string
}
