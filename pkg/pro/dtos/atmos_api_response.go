package dtos

// AtmosApiResponse is the canonical envelope for all Atmos Pro API responses.
//
// The server returns the user-facing failure description in `errorMessage`
// (camelCase). Older deployments may instead populate `error`; both are
// unmarshaled and EffectiveErrorMessage prefers `errorMessage` when present.
//
// `data.validationErrors` carries a list of granular failure reasons (e.g.
// per-rule drift-detection violations) that are rendered as bullets to the
// user instead of being smushed into a single sentence.
type AtmosApiResponse struct {
	Request      string                 `json:"request"`
	Status       int                    `json:"status"`
	Success      bool                   `json:"success"`
	ErrorTag     string                 `json:"errorTag,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
	TraceID      string                 `json:"traceId,omitempty"`
	Data         *AtmosApiResponseData  `json:"data,omitempty"`
}

// AtmosApiResponseData carries auxiliary structured data on error responses.
// Typed responses that embed AtmosApiResponse and define their own `Data`
// field shadow this one (Go field promotion + JSON unmarshaling rules).
type AtmosApiResponseData struct {
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

// EffectiveErrorMessage returns ErrorMessage when populated, otherwise Error.
// This tolerates both the current server format (errorMessage) and legacy or
// future deployments that may use the simpler `error` field.
func (r *AtmosApiResponse) EffectiveErrorMessage() string {
	if r.ErrorMessage != "" {
		return r.ErrorMessage
	}
	return r.Error
}
