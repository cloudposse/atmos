package exec

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
	"github.com/spf13/cobra"
)

// ProLockUnlockCmdArgs holds the common arguments for pro lock and unlock commands.
type ProLockUnlockCmdArgs struct {
	Component   string
	Stack       string
	AtmosConfig schema.AtmosConfiguration
}

// ProLockCmdArgs holds the arguments for the pro lock command.
type ProLockCmdArgs struct {
	ProLockUnlockCmdArgs
	LockMessage string
	LockTTL     int32
}

// ProUnlockCmdArgs holds the arguments for the pro unlock command.
type ProUnlockCmdArgs struct {
	ProLockUnlockCmdArgs
}

func parseLockUnlockCliArgs(cmd *cobra.Command, args []string) (ProLockUnlockCmdArgs, error) {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return ProLockUnlockCmdArgs{}, errors.Join(errUtils.ErrFailedToProcessArgs, err)
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return ProLockUnlockCmdArgs{}, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	flags := cmd.Flags()

	component, err := flags.GetString("component")
	if err != nil {
		return ProLockUnlockCmdArgs{}, errors.Join(errUtils.ErrFailedToGetComponentFlag, err)
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		return ProLockUnlockCmdArgs{}, errors.Join(errUtils.ErrFailedToGetStackFlag, err)
	}

	if component == "" || stack == "" {
		return ProLockUnlockCmdArgs{}, errUtils.ErrComponentAndStackRequired
	}

	result := ProLockUnlockCmdArgs{
		Component:   component,
		Stack:       stack,
		AtmosConfig: atmosConfig,
	}

	return result, nil
}

func parseLockCliArgs(cmd *cobra.Command, args []string) (ProLockCmdArgs, error) {
	commonArgs, err := parseLockUnlockCliArgs(cmd, args)
	if err != nil {
		return ProLockCmdArgs{}, err
	}

	flags := cmd.Flags()

	ttl, err := flags.GetInt32("ttl")
	if err != nil {
		return ProLockCmdArgs{}, err
	}

	if ttl == 0 {
		ttl = 30
	}

	message, err := flags.GetString("message")
	if err != nil {
		return ProLockCmdArgs{}, err
	}

	if message == "" {
		message = "Locked by Atmos"
	}

	result := ProLockCmdArgs{
		ProLockUnlockCmdArgs: commonArgs,
		LockMessage:          message,
		LockTTL:              ttl,
	}

	return result, nil
}

func parseUnlockCliArgs(cmd *cobra.Command, args []string) (ProUnlockCmdArgs, error) {
	commonArgs, err := parseLockUnlockCliArgs(cmd, args)
	if err != nil {
		return ProUnlockCmdArgs{}, err
	}

	result := ProUnlockCmdArgs{
		ProLockUnlockCmdArgs: commonArgs,
	}

	return result, nil
}

// ExecuteProLockCommand executes `atmos pro lock` command.
func ExecuteProLockCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteProLockCommand")()

	a, err := parseLockCliArgs(cmd, args)
	if err != nil {
		return err
	}

	gitRepo := git.NewDefaultGitRepo()

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(&a.AtmosConfig)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToCreateAPIClient, err)
	}

	return executeProLock(&a, apiClient, gitRepo)
}

// executeProLock is the core lock logic extracted for testability.
func executeProLock(a *ProLockCmdArgs, apiClient pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error {
	repoInfo, err := gitRepo.GetLocalRepoInfo()
	if err != nil {
		return errors.Join(errUtils.ErrFailedToGetLocalRepo, err)
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := dtos.LockStackRequest{
		Key:         fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
		TTL:         a.LockTTL,
		LockMessage: a.LockMessage,
		Properties:  nil,
	}

	lock, err := apiClient.LockStack(&dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToLockStack, err)
	}

	u.PrintfMessageToTUI("\n%s Stack '%s' successfully locked\n\n", theme.Styles.Checkmark, lock.Data.Key)
	log.Debug("Stack lock acquired", "key", lock.Data.Key, "lockID", lock.Data.ID, "expires", lock.Data.ExpiresAt)

	return nil
}

// ExecuteProUnlockCommand executes `atmos pro unlock` command.
func ExecuteProUnlockCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteProUnlockCommand")()

	a, err := parseUnlockCliArgs(cmd, args)
	if err != nil {
		return err
	}

	gitRepo := git.NewDefaultGitRepo()

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(&a.AtmosConfig)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToCreateAPIClient, err)
	}

	return executeProUnlock(&a, apiClient, gitRepo)
}

// executeProUnlock is the core unlock logic extracted for testability.
func executeProUnlock(a *ProUnlockCmdArgs, apiClient pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error {
	repoInfo, err := gitRepo.GetLocalRepoInfo()
	if err != nil {
		return errors.Join(errUtils.ErrFailedToGetLocalRepo, err)
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := dtos.UnlockStackRequest{
		Key: fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
	}

	_, err = apiClient.UnlockStack(&dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToUnlockStack, err)
	}

	u.PrintfMessageToTUI("\n%s Stack '%s' successfully unlocked\n\n", theme.Styles.Checkmark, dto.Key)
	log.Debug("Stack lock released", "key", dto.Key)

	return nil
}

// uploadStatus uploads the terraform results to the pro API.
func uploadStatus(info *schema.ConfigAndStacksInfo, exitCode int, metrics *process.ProcessMetrics, client pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error {
	// Get the git repository info.
	repoInfo, err := gitRepo.GetLocalRepoInfo()
	if err != nil {
		return errors.Join(errUtils.ErrFailedToGetLocalRepo, err)
	}

	// Get current git SHA.
	gitSHA, err := gitRepo.GetCurrentCommitSHA()
	if err != nil {
		// Log warning but don't fail the upload.
		log.Warn(fmt.Sprintf("Failed to get current git SHA: %v", err))
		gitSHA = ""
	}

	// Get run ID from environment variables.
	// Note: This is an exception to the general rule of using viper.BindEnv for environment variables.
	// The run ID is always provided by the CI/CD environment and is not part of the stack configuration.
	//nolint:forbidigo // Exception: Run ID is always from CI/CD environment, not config
	atmosProRunID := os.Getenv("ATMOS_PRO_RUN_ID")

	// Create the DTO.
	dto := dtos.InstanceStatusUploadRequest{
		AtmosProRunID: atmosProRunID,
		AtmosVersion:  pkgversion.Version,
		AtmosOS:       runtime.GOOS,
		AtmosArch:     runtime.GOARCH,
		GitSHA:        gitSHA,
		RepoURL:       repoInfo.RepoUrl,
		RepoName:      repoInfo.RepoName,
		RepoOwner:     repoInfo.RepoOwner,
		RepoHost:      repoInfo.RepoHost,
		Stack:         info.Stack,
		Component:     info.Component,
		Command:       info.SubCommand,
		ExitCode:      exitCode,
	}

	// Add last_run timestamp if we have a run ID or git SHA.
	if atmosProRunID != "" || gitSHA != "" {
		dto.LastRun = time.Now().UTC().Format(time.RFC3339)
	}

	// Populate resource metrics if available.
	if metrics != nil {
		populateMetricsDTO(&dto, metrics)
	}

	// Upload the status.
	if err := client.UploadInstanceStatus(&dto); err != nil {
		return errors.Join(errUtils.ErrFailedToUploadInstanceStatus, err)
	}

	return nil
}

// populateMetricsDTO fills the DTO with resource metrics from a completed subprocess.
func populateMetricsDTO(dto *dtos.InstanceStatusUploadRequest, m *process.ProcessMetrics) {
	wallMs := m.WallTime.Milliseconds()
	userMs := m.UserCPUTime.Milliseconds()
	sysMs := m.SystemCPUTime.Milliseconds()
	dto.WallTimeMs = &wallMs
	dto.UserCPUTimeMs = &userMs
	dto.SysCPUTimeMs = &sysMs

	// Rusage fields — only populate if non-zero (unavailable on Windows).
	if m.MaxRSSBytes > 0 {
		dto.PeakRSSBytes = &m.MaxRSSBytes
	}
	if m.MinorPageFaults > 0 {
		dto.MinorPageFaults = &m.MinorPageFaults
	}
	if m.MajorPageFaults > 0 {
		dto.MajorPageFaults = &m.MajorPageFaults
	}
	if m.InBlockOps > 0 {
		dto.InBlockOps = &m.InBlockOps
	}
	if m.OutBlockOps > 0 {
		dto.OutBlockOps = &m.OutBlockOps
	}
	if m.VolCtxSwitches > 0 {
		dto.VolCtxSwitches = &m.VolCtxSwitches
	}
	if m.InvolCtxSwitches > 0 {
		dto.InvolCtxSwitches = &m.InvolCtxSwitches
	}
}

// shouldUploadStatus determines if status should be uploaded.
func shouldUploadStatus(info *schema.ConfigAndStacksInfo) bool {
	// Only upload for plan and apply commands.
	if info.SubCommand != "plan" && info.SubCommand != "apply" {
		return false
	}

	// Check if pro is enabled in component settings
	if proSettings, ok := info.ComponentSettingsSection["pro"].(map[string]interface{}); ok {
		if enabled, ok := proSettings["enabled"].(bool); ok && enabled {
			return true
		}
	}

	// Log warning if pro is not enabled
	log.Warn("Pro is not enabled. Skipping upload of Terraform result.")

	return false
}
