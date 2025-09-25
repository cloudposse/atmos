package dtos

type AtmosApiResponse struct {
	Request      string                 `json:"request"`
	Status       int                    `json:"status"`
	Success      bool                   `json:"success"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
	TraceID      string                 `json:"traceId,omitempty"`
}
