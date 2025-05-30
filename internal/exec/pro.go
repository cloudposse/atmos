package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	git "github.com/cloudposse/atmos/pkg/git"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

type ProLockUnlockCmdArgs struct {
	Component string
	Logger    *l.Logger
	Stack     string
}

type ProLockCmdArgs struct {
	ProLockUnlockCmdArgs
	LockMessage string
	LockTTL     int32
}

type ProUnlockCmdArgs struct {
	ProLockUnlockCmdArgs
}

// GitRepoInterface defines the interface for git repository operations
type GitRepoInterface interface {
	GetLocalRepo() (*git.RepoInfo, error)
	GetRepoInfo(repo *git.RepoInfo) (git.RepoInfo, error)
}

// DefaultGitRepo is the default implementation of GitRepoInterface
type DefaultGitRepo struct{}

func (d *DefaultGitRepo) GetLocalRepo() (*git.RepoInfo, error) {
	repo, err := git.GetLocalRepo()
	if err != nil {
		return nil, err
	}
	info, err := git.GetRepoInfo(repo)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *DefaultGitRepo) GetRepoInfo(repo *git.RepoInfo) (git.RepoInfo, error) {
	return *repo, nil
}

func parseLockUnlockCliArgs(cmd *cobra.Command, args []string) (ProLockUnlockCmdArgs, error) {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return ProLockUnlockCmdArgs{}, err
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return ProLockUnlockCmdArgs{}, err
	}

	logger, err := l.NewLoggerFromCliConfig(atmosConfig)
	if err != nil {
		return ProLockUnlockCmdArgs{}, err
	}

	flags := cmd.Flags()

	component, err := flags.GetString("component")
	if err != nil {
		return ProLockUnlockCmdArgs{}, err
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		return ProLockUnlockCmdArgs{}, err
	}

	if component == "" || stack == "" {
		return ProLockUnlockCmdArgs{}, fmt.Errorf("both '--component' and '--stack' flag must be provided")
	}

	result := ProLockUnlockCmdArgs{
		Component: component,
		Logger:    logger,
		Stack:     stack,
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
		return err
	}

	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		return err
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := pro.LockStackRequest{
		Key:         fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
		TTL:         a.LockTTL,
		LockMessage: a.LockMessage,
		Properties:  nil,
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(a.Logger)
	if err != nil {
		return err
	}

	lock, err := apiClient.LockStack(dto)
	if err != nil {
		return err
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
		return err
	}

	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		return err
	}

	owner := repoInfo.RepoOwner
	repoName := repoInfo.RepoName

	dto := pro.UnlockStackRequest{
		Key: fmt.Sprintf("%s/%s/%s/%s", owner, repoName, a.Stack, a.Component),
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(a.Logger)
	if err != nil {
		return err
	}

	_, err = apiClient.UnlockStack(dto)
	if err != nil {
		return err
	}

	a.Logger.Info(fmt.Sprintf("Key '%s' successfully unlocked.\n", dto.Key))

	return nil
}

// uploadDriftResultWithClient uploads the terraform results to the pro API
// It takes a mock client for testing purposes
func uploadDriftResultWithClient(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, exitCode int, client pro.AtmosProAPIClientInterface, gitRepo GitRepoInterface) error {
	// Get the local repository
	repo, err := gitRepo.GetLocalRepo()
	if err != nil {
		return err
	}

	// Get repository info
	repoInfo, err := gitRepo.GetRepoInfo(repo)
	if err != nil {
		return err
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
	return client.UploadDriftResultStatus(dto)
}

// uploadDriftResult uploads the terraform results to the pro API
func uploadDriftResult(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, exitCode int) error {
	// Only upload if exit code is 0 (no changes) or 2 (changes)
	if exitCode != 0 && exitCode != 2 {
		return nil
	}

	// Initialize the API client
	client, err := pro.NewAtmosProAPIClientFromEnv(nil)
	if err != nil {
		return err
	}

	// Use the default git repo implementation
	gitRepo := &DefaultGitRepo{}

	// Upload the drift result
	return uploadDriftResultWithClient(atmosConfig, info, exitCode, client, gitRepo)
}

// shouldUploadDriftResult determines if drift results should be uploaded
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
