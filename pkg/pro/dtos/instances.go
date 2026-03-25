package dtos

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// InstancesUploadRequest represents the data structure for uploading components for drift detection.
// We call this from "atmos list instances".
type InstancesUploadRequest struct {
	RepoURL   string            `json:"repo_url"`
	RepoName  string            `json:"repo_name"`
	RepoOwner string            `json:"repo_owner"`
	RepoHost  string            `json:"repo_host"`
	Instances []schema.Instance `json:"instances"`
}

// InstanceStatusUploadRequest represents the data structure for uploading a single instance's status.
type InstanceStatusUploadRequest struct {
	// Identity and context.
	AtmosProRunID string `json:"atmos_pro_run_id,omitempty"`
	AtmosVersion  string `json:"atmos_version,omitempty"`
	AtmosOS       string `json:"atmos_os,omitempty"`
	AtmosArch     string `json:"atmos_arch,omitempty"`
	GitSHA        string `json:"git_sha,omitempty"`
	RepoURL       string `json:"repo_url,omitempty"`
	RepoName      string `json:"repo_name"`
	RepoOwner     string `json:"repo_owner"`
	RepoHost      string `json:"repo_host,omitempty"`
	Component     string `json:"component"`
	Stack         string `json:"stack"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	LastRun       string `json:"last_run,omitempty"`

	// Resource metrics (timing).
	WallTimeMs    *int64 `json:"wall_time_ms,omitempty"`
	UserCPUTimeMs *int64 `json:"user_cpu_time_ms,omitempty"`
	SysCPUTimeMs  *int64 `json:"sys_cpu_time_ms,omitempty"`

	// Resource metrics (memory).
	PeakRSSBytes *int64 `json:"peak_rss_bytes,omitempty"`

	// Resource metrics (page faults).
	MinorPageFaults *int64 `json:"minor_page_faults,omitempty"`
	MajorPageFaults *int64 `json:"major_page_faults,omitempty"`

	// Resource metrics (I/O).
	InBlockOps  *int64 `json:"in_block_ops,omitempty"`
	OutBlockOps *int64 `json:"out_block_ops,omitempty"`

	// Resource metrics (context switches).
	VolCtxSwitches   *int64 `json:"vol_ctx_switches,omitempty"`
	InvolCtxSwitches *int64 `json:"invol_ctx_switches,omitempty"`
}
