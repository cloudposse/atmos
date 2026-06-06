package dtos

// Envelope wraps any typed Atmos Pro response payload in the canonical envelope so
// callers read the payload from `data`, never the top level. See AtmosApiResponse for
// the envelope fields (success/status/errorMessage/...).
//
// This is the generic form of the per-call shadow-Data DTOs (e.g.
// ExchangeGitHubOIDCTokenResponse): embedding AtmosApiResponse promotes its fields while
// the typed Data field shadows the embedded *AtmosApiResponseData so the payload decodes
// from `data` into T.
type Envelope[T any] struct {
	AtmosApiResponse
	Data T `json:"data"`
}
