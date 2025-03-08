package exec

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
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

	logger, err := l.InitializeLoggerFromCliConfig(&atmosConfig)
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
