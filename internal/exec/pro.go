package exec

import (
	"errors"
	"fmt"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	git "github.com/cloudposse/atmos/pkg/git"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// Error variables for pro package.
var (
	ErrComponentAndStackRequired = errors.New("both '--component' and '--stack' flag must be provided")
	ErrFailedToGetLocalRepo      = errors.New("failed to get local repository")
	ErrFailedToGetRepoInfo       = errors.New("failed to get repository info")
	ErrFailedToCreateAPIClient   = errors.New("failed to create API client")
	ErrFailedToProcessArgs       = errors.New("failed to process command line arguments")
	ErrFailedToInitConfig        = errors.New("failed to initialize CLI config")
	ErrFailedToCreateLogger      = errors.New("failed to create logger")
	ErrFailedToGetComponentFlag  = errors.New("failed to get component flag")
	ErrFailedToGetStackFlag      = errors.New("failed to get stack flag")
)

type ProLockUnlockCmdArgs struct {
	Component   string
	Logger      *l.Logger
	Stack       string
	AtmosConfig schema.AtmosConfiguration
}

type ProLockCmdArgs struct {
	ProLockUnlockCmdArgs
	LockMessage string
	LockTTL     int32
}

type ProUnlockCmdArgs struct {
	ProLockUnlockCmdArgs
}

func parseLockUnlockCliArgs(cmd *cobra.Command, args []string) (ProLockUnlockCmdArgs, error) {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return ProLockUnlockCmdArgs{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToProcessArgs, err)
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return ProLockUnlockCmdArgs{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToInitConfig, err)
	}

	logger, err := l.NewLoggerFromCliConfig(atmosConfig)
	if err != nil {
		return ProLockUnlockCmdArgs{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateLogger, err)
	}

	flags := cmd.Flags()

	component, err := flags.GetString("component")
	if err != nil {
		return ProLockUnlockCmdArgs{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetComponentFlag, err)
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		return ProLockUnlockCmdArgs{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetStackFlag, err)
	}

	if component == "" || stack == "" {
		return ProLockUnlockCmdArgs{}, ErrComponentAndStackRequired
	}

	result := ProLockUnlockCmdArgs{
		Component:   component,
		Logger:      logger,
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

// ExecuteProLockCommand executes `atmos pro lock` command
func ExecuteProLockCommand(cmd *cobra.Command, args []string) error {
	a, err := parseLockCliArgs(cmd, args)
	if err != nil {
		return err
	}

	repo, err := git.GetLocalRepo()
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetLocalRepo, err)
	}

	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetRepoInfo, err)
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := dtos.LockStackRequest{
		Key:         fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
		TTL:         a.LockTTL,
		LockMessage: a.LockMessage,
		Properties:  nil,
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(a.Logger, &a.AtmosConfig)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAPIClient, err)
	}

	lock, err := apiClient.LockStack(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, pro.ErrFailedToLockStack, err)
	}

	a.Logger.Info("Stack successfully locked.\n")
	a.Logger.Info(fmt.Sprintf("Key: %s", lock.Data.Key))
	a.Logger.Info(fmt.Sprintf("LockID: %s", lock.Data.ID))
	a.Logger.Info(fmt.Sprintf("Expires %s", lock.Data.ExpiresAt))

	return nil
}

// ExecuteProUnlockCommand executes `atmos pro unlock` command
func ExecuteProUnlockCommand(cmd *cobra.Command, args []string) error {
	a, err := parseUnlockCliArgs(cmd, args)
	if err != nil {
		return err
	}

	repo, err := git.GetLocalRepo()
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetLocalRepo, err)
	}

	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetRepoInfo, err)
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := dtos.UnlockStackRequest{
		Key: fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(a.Logger, &a.AtmosConfig)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAPIClient, err)
	}

	_, err = apiClient.UnlockStack(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, pro.ErrFailedToUnlockStack, err)
	}

	a.Logger.Info(fmt.Sprintf("Key '%s' successfully unlocked.\n", dto.Key))

	return nil
}

// uploadDriftResult uploads the terraform results to the pro API.
func uploadDriftResult(info *schema.ConfigAndStacksInfo, exitCode int, client pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error {
	// Only upload if exit code is 0 (no changes) or 2 (changes)
	if exitCode != 0 && exitCode != 2 {
		return nil
	}

	// Get the git repository info
	repoInfo, err := gitRepo.GetLocalRepo()
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetLocalRepo, err)
	}

	// Create the DTO
	dto := pro.DriftStatusUploadRequest{
		RepoURL:   repoInfo.RepoUrl,
		RepoName:  repoInfo.RepoName,
		RepoOwner: repoInfo.RepoOwner,
		RepoHost:  repoInfo.RepoHost,
		Stack:     info.Stack,
		Component: info.Component,
		HasDrift:  exitCode == 2,
	}

	// Upload the drift result status
	if err := client.UploadDriftResultStatus(&dto); err != nil {
		return fmt.Errorf(cfg.ErrFormatString, pro.ErrFailedToUploadDriftStatus, err)
	}

	return nil
}

// shouldUploadDriftResult determines if drift results should be uploaded.
func shouldUploadDriftResult(info *schema.ConfigAndStacksInfo) bool {
	// Only upload for plan command
	if info.SubCommand != "plan" {
		return false
	}

	// Check if pro is enabled
	if proSettings, ok := info.ComponentSettingsSection["pro"].(map[string]interface{}); ok {
		if enabled, ok := proSettings["enabled"].(bool); ok && enabled {
			return true
		}
	}

	// Create logger from atmos config
	log.Warn("Pro is not enabled. Skipping upload of Terraform result.")

	return false
}
