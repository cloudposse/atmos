package dtos

import "encoding/json"

// ResourceUsageMetrics captures how much time and system resources a command
// consumed while running. This is an allowlist — new fields must be explicitly
// added here. Unix-only fields are omitted on platforms without support.
type ResourceUsageMetrics struct {
	WallTimeMS       int64 `json:"wall_time_ms"`
	UserCPUTimeMS    int64 `json:"user_cpu_time_ms"`
	SystemCPUTimeMS  int64 `json:"system_cpu_time_ms"`
	MaxRSSBytes      int64 `json:"max_rss_bytes,omitempty"`
	MinorPageFaults  int64 `json:"minor_page_faults,omitempty"`
	MajorPageFaults  int64 `json:"major_page_faults,omitempty"`
	InBlockOps       int64 `json:"in_block_ops,omitempty"`
	OutBlockOps      int64 `json:"out_block_ops,omitempty"`
	VolCtxSwitches   int64 `json:"vol_ctx_switches,omitempty"`
	InvolCtxSwitches int64 `json:"invol_ctx_switches,omitempty"`
}

// ExecUploadRequest represents the data structure for uploading a single
// command-execution record to Atmos Pro. This is an allowlist — new fields
// must be explicitly added here. Sensitive data is masked before this struct
// is marshaled (see pkg/proexec).
type ExecUploadRequest struct {
	// ExecutionID uniquely identifies this single execution record — a fresh
	// UUID v4 generated once per qualifying invocation. Distinct from
	// AtmosProRunID, which correlates records across a whole CI run. Also
	// sent as ExecDataUploadRequest.ExecutionID when Data requires
	// out-of-band delivery, so Atmos Pro can associate the two (FR-003c).
	ExecutionID   string `json:"execution_id"`
	AtmosProRunID string `json:"atmos_pro_run_id"`
	AtmosVersion  string `json:"atmos_version"`
	AtmosOS       string `json:"atmos_os"`
	AtmosArch     string `json:"atmos_arch"`
	Command       string `json:"command"`
	// Args holds only the invocation's positional arguments (e.g. the
	// component identifier). CLI flags are reported separately in Flags —
	// the two are never combined into one array (FR-003b).
	Args []string `json:"args"`
	// Flags holds the CLI flags actually passed (e.g. "-s", "plat-use2-dev"),
	// masked the same way Args is. Kept distinct from Args per FR-003b so the
	// two remain correlatable in content with the older, independent
	// uploadStatus mechanism (internal/exec/pro.go) without merging shapes.
	Flags     []string             `json:"flags"`
	ExitCode  int                  `json:"exit_code"`
	GitSHA    string               `json:"git_sha"`
	RepoURL   string               `json:"repo_url"`
	RepoName  string               `json:"repo_name"`
	RepoOwner string               `json:"repo_owner"`
	RepoHost  string               `json:"repo_host"`
	Metrics   ResourceUsageMetrics `json:"metrics"`
	// Data is command-specific structured data (e.g. terraform plan/apply
	// resource counts, outputs, warnings, and per-resource change lists).
	// Absent (nil) for commands with no structured-data extension, per
	// FR-005/data-model.md. On the wire it is always exactly one of two
	// shapes: an inline JSON structure (object/array), when the whole
	// marshaled record is under the payload size threshold; or a JSON
	// string holding a blob URL returned by POST /v1/atmos/exec/data, when
	// the whole record is at/over the threshold (FR-011). Never chunked —
	// UploadExecMetadata decides which shape to send, never both.
	Data json.RawMessage `json:"data,omitempty"`
}

// ExecUploadResponse represents the response from POST /v1/atmos/exec.
type ExecUploadResponse struct {
	AtmosApiResponse
}

// ExecDataUploadRequest is the request body for POST /v1/atmos/exec/data,
// used to upload a command's structured Data out-of-band, in a single
// request (never chunked), when the parent ExecUploadRequest would otherwise
// exceed the payload size threshold (FR-011).
type ExecDataUploadRequest struct {
	// ExecutionID MUST match the corresponding ExecUploadRequest.ExecutionID,
	// so Atmos Pro can associate this blob with that execution record.
	ExecutionID string          `json:"execution_id"`
	Data        json.RawMessage `json:"data"`
}

// ExecDataUploadResponse represents the response from POST
// /v1/atmos/exec/data. URL is the blob's retrievable location, to be set as
// the corresponding ExecUploadRequest.Data content (as a JSON string) on the
// subsequent POST /v1/atmos/exec request.
type ExecDataUploadResponse struct {
	AtmosApiResponse
	URL string `json:"url"`
}
