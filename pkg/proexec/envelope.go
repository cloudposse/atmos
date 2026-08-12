package proexec

import (
	"encoding/json"
	"os"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	git "github.com/cloudposse/atmos/pkg/git"
	io "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
)

// atmosProRunIDEnvVar is the CI-provided correlation ID reused from other
// Atmos Pro uploads (see internal/exec/pro.go: uploadStatus).
const atmosProRunIDEnvVar = "ATMOS_PRO_RUN_ID"

// buildRecord assembles the base execution-record envelope (version, OS,
// arch, command path, ATMOS_PRO_RUN_ID, git info, resource-usage metrics),
// and applies secret masking to Args, data, and dataItems (FR-010). A nil
// data/dataItems argument produces a request with that field entirely absent
// from the marshaled JSON. Payload-size handling (FR-011) is not performed
// here — oversized dataItems are split across multiple correlated requests
// by pro.UploadExecMetadata, never truncated or dropped.
func buildRecord(
	command string,
	exitCode int,
	metrics process.ProcessMetrics,
	data any,
	dataItems []any,
	gitRepo git.GitRepoInterface,
) (*dtos.ExecUploadRequest, error) {
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

	maskedArgs := maskArgs(nil)

	dataRaw, err := maskedDataJSON(data)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToUploadExecMetadata).WithCause(err).Err()
	}

	dataItemsRaw, err := maskedDataItemsJSON(dataItems)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToUploadExecMetadata).WithCause(err).Err()
	}

	req := &dtos.ExecUploadRequest{
		AtmosProRunID: atmosProRunID,
		AtmosVersion:  pkgversion.Version,
		AtmosOS:       runtime.GOOS,
		AtmosArch:     runtime.GOARCH,
		Command:       command,
		Args:          maskedArgs,
		ExitCode:      exitCode,
		GitSHA:        gitSHA,
		RepoURL:       repoInfo.RepoUrl,
		RepoName:      repoInfo.RepoName,
		RepoOwner:     repoInfo.RepoOwner,
		RepoHost:      repoInfo.RepoHost,
		Metrics:       toResourceUsageMetrics(metrics),
		Data:          dataRaw,
		DataItems:     dataItemsRaw,
	}

	return req, nil
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

// maskedDataItemsJSON marshals each item in dataItems to JSON and runs the
// result through the existing secret-masking path before it becomes part of
// the upload payload (FR-010). A nil/empty dataItems argument returns a nil
// slice so the DataItems field is omitted entirely from the marshaled
// request.
func maskedDataItemsJSON(dataItems []any) ([]json.RawMessage, error) {
	if len(dataItems) == 0 {
		return nil, nil
	}

	out := make([]json.RawMessage, len(dataItems))
	for i, item := range dataItems {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		out[i] = json.RawMessage(io.MaskString(string(raw)))
	}
	return out, nil
}

// toResourceUsageMetrics converts process.ProcessMetrics to the DTO shape.
func toResourceUsageMetrics(m process.ProcessMetrics) dtos.ResourceUsageMetrics {
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
