package dtos

// ExchangeGitHubOIDCTokenRequest represents the request to exchange OIDC token for Atmos token.
type ExchangeGitHubOIDCTokenRequest struct {
	Token       string `json:"token"`
	WorkspaceID string `json:"workspaceId"`
}

// ExchangeGitHubOIDCTokenResponse represents the response from Atmos Pro's OIDC auth endpoint.
type ExchangeGitHubOIDCTokenResponse struct {
	AtmosApiResponse
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}
