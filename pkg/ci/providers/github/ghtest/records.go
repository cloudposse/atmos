package ghtest

// Comment is an issue or pull request comment.
type Comment struct {
	// Owner is the repository owner.
	Owner string
	// Repo is the repository name.
	Repo string
	// Number is the issue or pull request number the comment belongs to.
	Number int
	// ID is the comment ID. Zero on a seeded comment means "assign one".
	ID int64
	// Body is the comment markdown.
	Body string
	// HTMLURL is the comment permalink assigned by the server.
	HTMLURL string
	// Edited is true when this record was produced by an edit (PATCH) rather than a create.
	Edited bool
}

// Status is a commit status written through POST /repos/{owner}/{repo}/statuses/{sha}.
type Status struct {
	Owner       string
	Repo        string
	SHA         string
	State       string
	Context     string
	Description string
	TargetURL   string
}

// SARIFUpload is a code-scanning upload received by POST /repos/{owner}/{repo}/code-scanning/sarifs.
type SARIFUpload struct {
	Owner     string
	Repo      string
	CommitSHA string
	Ref       string
	// SARIF is the decoded SARIF document (the "sarif" field is base64 of gzip).
	SARIF string
	// ID is the analysis ID the server returned.
	ID string
}

// RecordedRequest is a raw HTTP request the server saw, including unexpected ones.
type RecordedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Body     string
	// Status is the HTTP status code the server answered with. It is zero while the
	// handler is still running: a request is listed as soon as it arrives.
	Status int
}

// CheckRun is a check run returned by the commit check-runs endpoint.
type CheckRun struct {
	Name       string
	Status     string
	Conclusion string
	DetailsURL string
}

// PullRequest is a pull request returned by the pulls endpoints.
type PullRequest struct {
	Number  int
	Title   string
	HeadRef string
	HeadSHA string
	BaseRef string
	HTMLURL string
}
