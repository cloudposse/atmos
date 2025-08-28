package dtos

// GetGitHubOIDCResponse represents the response from GitHub's OIDC token endpoint when requesting an OIDC token.
type GetGitHubOIDCResponse struct {
	Value string `json:"value"`
}
