package dtos

import "encoding/json"

// SecurityFindingsUploadRequest is the payload for uploading AWS security
// findings (as SARIF) to Atmos Pro. The SARIF document is sent as-is so the
// server can ingest it without re-parsing Atmos-specific structures.
//
// The exact server-side contract is pending. The "experimental" flag on the
// CLI side reflects that callers may need to update fields here once the
// endpoint contract is finalized.
type SecurityFindingsUploadRequest struct {
	RepoURL   string          `json:"repo_url"`
	RepoName  string          `json:"repo_name"`
	RepoOwner string          `json:"repo_owner"`
	RepoHost  string          `json:"repo_host"`
	GitSHA    string          `json:"git_sha,omitempty"`
	Stack     string          `json:"stack,omitempty"`
	Component string          `json:"component,omitempty"`
	Format    string          `json:"format"`
	SARIF     json.RawMessage `json:"sarif"`
}
