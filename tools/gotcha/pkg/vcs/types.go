package vcs

// CommentMetadata contains common metadata for VCS comments.
type CommentMetadata struct {
	UUID      string
	Platform  Platform
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