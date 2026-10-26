package dtos

// CommitFileAddition represents a file to add or modify in the commit.
type CommitFileAddition struct {
	Path     string `json:"path"`
	Contents string `json:"contents"` // base64-encoded file contents.
}

// CommitFileDeletion represents a file to delete in the commit.
type CommitFileDeletion struct {
	Path string `json:"path"`
}

// CommitChanges groups file additions and deletions for a commit.
type CommitChanges struct {
	Additions []CommitFileAddition `json:"additions"`
	Deletions []CommitFileDeletion `json:"deletions"`
}

// CommitRequest is the request body for POST /api/v1/git/commit.
type CommitRequest struct {
	Branch        string        `json:"branch"`
	Changes       CommitChanges `json:"changes"`
	CommitMessage string        `json:"commitMessage"`
	Comment       string        `json:"comment,omitempty"`
}

// CommitResponse is the response from POST /api/v1/git/commit.
type CommitResponse struct {
	AtmosApiResponse
	Data struct {
		SHA string `json:"sha"`
	} `json:"data"`
}
