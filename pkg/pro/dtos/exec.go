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
	AtmosProRunID string               `json:"atmos_pro_run_id"`
	AtmosVersion  string               `json:"atmos_version"`
	AtmosOS       string               `json:"atmos_os"`
	AtmosArch     string               `json:"atmos_arch"`
	Command       string               `json:"command"`
	Args          []string             `json:"args"`
	ExitCode      int                  `json:"exit_code"`
	GitSHA        string               `json:"git_sha"`
	RepoURL       string               `json:"repo_url"`
	RepoName      string               `json:"repo_name"`
	RepoOwner     string               `json:"repo_owner"`
	RepoHost      string               `json:"repo_host"`
	Metrics       ResourceUsageMetrics `json:"metrics"`
	// Data is command-specific structured *summary* data (e.g. terraform
	// plan/apply resource counts, outputs, warnings). Small and bounded —
	// always sent in full, never chunked. Absent (nil) for commands with no
	// structured-data extension, per FR-005/data-model.md.
	Data json.RawMessage `json:"data,omitempty"`
	// DataItems is command-specific structured *bulk* data (e.g. one entry
	// per terraform plan/apply resource change). Potentially large — split
	// across multiple correlated requests via chunking rather than truncated
	// or dropped when it would exceed the payload size limit (FR-011).
	DataItems []json.RawMessage `json:"data_items,omitempty"`
	// BatchID/BatchIndex/BatchTotal are present only when DataItems was
	// split across multiple requests, correlating the chunks for server-side
	// reassembly (mirrors UploadAffectedStacksRequest/InstancesUploadRequest).
	BatchID    string `json:"batch_id,omitempty"`
	BatchIndex *int   `json:"batch_index,omitempty"`
	BatchTotal *int   `json:"batch_total,omitempty"`
}

// ExecUploadResponse represents the response from POST /v1/atmos/exec.
type ExecUploadResponse struct {
	AtmosApiResponse
}
