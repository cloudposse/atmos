package proexec

import (
	"encoding/json"
	"os"
	"runtime"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	git "github.com/cloudposse/atmos/pkg/git"
	io "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
)

// atmosProRunIDEnvVar is the CI-provided correlation ID reused from other
// Atmos Pro uploads (see internal/exec/pro.go: uploadStatus).
const atmosProRunIDEnvVar = "ATMOS_PRO_RUN_ID"

// ExecRecordInput bundles the per-invocation fields buildRecord and
// uploadExecMetadata need beyond the process-wide metrics/gitRepo
// dependencies — Command, Args, Flags, ExitCode, and Data — grouped to stay
// under the linter's argument-count limit. Args MUST hold only positional
// arguments and flags MUST hold only CLI flags — the two are kept in
// separate fields, never combined (FR-003b).
type ExecRecordInput struct {
	Command  string
	Args     []string
	Flags    []string
	ExitCode int
	Data     any
}

// buildRecord assembles the base execution-record envelope (ExecutionID,
// version, OS, arch, command path, ATMOS_PRO_RUN_ID, git info, resource-usage
// metrics), and applies secret masking to Args, Flags, and data (FR-010). A
// nil in.Data produces a request with Data entirely absent from the
// marshaled JSON. Payload-size handling (FR-011) is not performed here — an
// oversized data value is uploaded out-of-band by pro.UploadExecMetadata,
// never truncated or dropped.
func buildRecord(in *ExecRecordInput, metrics *process.ProcessMetrics, gitRepo git.GitRepoInterface) (*dtos.ExecUploadRequest, error) {
	repoInfo, err := gitRepo.GetLocalRepoInfo()
	if err != nil {
		log.Debug("Failed to get local repo info for exec-metadata upload.", "error", err)
		repoInfo = &git.RepoInfo{}
	}

	gitSHA, err := gitRepo.GetCurrentCommitSHA()
	if err != nil {
		log.Debug("Failed to get current git SHA for exec-metadata upload.", "error", err)
		gitSHA = ""
	}

	// Note: This is an exception to the general rule of using viper.BindEnv for
	// environment variables. The run ID is always provided by the CI/CD
	// environment and is not part of the stack configuration, matching
	// internal/exec/pro.go: uploadStatus's sourcing of the same value.
	//nolint:forbidigo // Exception: Run ID is always from CI/CD environment, not config
	atmosProRunID := os.Getenv(atmosProRunIDEnvVar)

	maskedArgs := maskArgs(in.Args)
	maskedFlags := maskArgs(in.Flags)

	dataRaw, err := maskedDataJSON(in.Data)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToUploadExecMetadata).WithCause(err).Err()
	}

	req := &dtos.ExecUploadRequest{
		ExecutionID:   uuid.New().String(),
		AtmosProRunID: atmosProRunID,
		AtmosVersion:  pkgversion.Version,
		AtmosOS:       runtime.GOOS,
		AtmosArch:     runtime.GOARCH,
		Command:       in.Command,
		Args:          maskedArgs,
		Flags:         maskedFlags,
		ExitCode:      in.ExitCode,
		GitSHA:        gitSHA,
		RepoURL:       repoInfo.RepoUrl,
		RepoName:      repoInfo.RepoName,
		RepoOwner:     repoInfo.RepoOwner,
		RepoHost:      repoInfo.RepoHost,
		Metrics:       toResourceUsageMetrics(metrics),
		Data:          dataRaw,
	}

	return req, nil
}

// VersionedData wraps payload under key, alongside a top-level "version"
// field, for the command-specific structured Data shapes that are a single
// key wrapping one payload (describe affected's {stacks}, list instances'
// {instances}). Every shape defined by this feature carries its own
// "version" — a plain integer starting at 1, incremented independently per
// shape — so Atmos Pro can validate/parse each command's structured data
// according to its declared shape version (FR-005a, research.md Decision
// 24). TerraformExecData's multi-key shape does not use this helper — it
// adds "version" directly to its own map literal instead.
func VersionedData(version int, key string, payload any) map[string]any {
	defer perf.Track(nil, "proexec.VersionedData")()

	return map[string]any{
		"version": version,
		key:       payload,
	}
}

// maskArgs runs the existing secret-masking path over each argument.
func maskArgs(args []string) []string {
	if len(args) == 0 {
		return []string{}
	}
	masked := make([]string, len(args))
	for i, a := range args {
		masked[i] = io.MaskString(a)
	}
	return masked
}

// maskedDataJSON marshals data (when non-nil) to JSON and runs the result
// through the existing secret-masking path before it becomes part of the
// upload payload (FR-010). A nil data argument returns a nil json.RawMessage
// so the Data field is omitted entirely from the marshaled request.
func maskedDataJSON(data any) (json.RawMessage, error) {
	if data == nil {
		return nil, nil
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	masked := io.MaskString(string(raw))
	return json.RawMessage(masked), nil
}

// toResourceUsageMetrics converts process.ProcessMetrics to the DTO shape.
func toResourceUsageMetrics(m *process.ProcessMetrics) dtos.ResourceUsageMetrics {
	return dtos.ResourceUsageMetrics{
		WallTimeMS:       m.WallTime.Milliseconds(),
		UserCPUTimeMS:    m.UserCPUTime.Milliseconds(),
		SystemCPUTimeMS:  m.SystemCPUTime.Milliseconds(),
		MaxRSSBytes:      m.MaxRSSBytes,
		MinorPageFaults:  m.MinorPageFaults,
		MajorPageFaults:  m.MajorPageFaults,
		InBlockOps:       m.InBlockOps,
		OutBlockOps:      m.OutBlockOps,
		VolCtxSwitches:   m.VolCtxSwitches,
		InvolCtxSwitches: m.InvolCtxSwitches,
	}
}
