package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authManagerFactory creates an AuthManager from the given parameters.
// Package-level variable to allow test injection.
var authManagerFactory = func(identity string, authConfig schema.AuthConfig, flagSelectValue string, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	mergedAuthConfig := auth.CopyGlobalAuthConfig(&authConfig)
	return auth.CreateAndAuthenticateManagerWithAtmosConfig(identity, mergedAuthConfig, flagSelectValue, atmosConfig)
}

// ExecuteTerraformQuery executes `atmos terraform <command> --query <yq-expression --stack <stack>`.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQuery")()
	return ExecuteTerraformQueryWithContext(context.Background(), info)
}

// ExecuteTerraformQueryWithContext executes graph-backed multi-component Terraform work.
func ExecuteTerraformQueryWithContext(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQueryWithContext")()
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Create auth manager for YAML function processing during stack description.
	// Without this, YAML functions like !terraform.state fail when using --all
	// because they cannot access authenticated credentials (e.g., AWS SSO).
	// Fixes: https://github.com/cloudposse/atmos/issues/2081
	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return err
	}

	// Inject auth resolver into identity-aware stores so they can lazily resolve
	// credentials on first access. This bridges the store system with the auth system.
	if authManager != nil {
		injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}

	stacks, err := ExecuteDescribeStacks(
		&atmosConfig,
		info.Stack,
		info.Components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
	)
	if err != nil {
		return err
	}

	return scheduleradapters.ExecuteTerraform(ctx, scheduleradapters.TerraformOptions{
		AtmosConfig: &atmosConfig,
		Info:        info,
		Stacks:      stacks,
		Executor:    executeTerraformQueryComponent,
	})
}

// createQueryAuthManager creates an AuthManager for multi-component execution paths.
// This is needed so that YAML functions (e.g., !terraform.state) can use authenticated
// credentials when ExecuteDescribeStacks processes stack configurations.
// Returns nil AuthManager (no error) if authentication is not configured.
func createQueryAuthManager(info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.createQueryAuthManager")()

	authManager, err := authManagerFactory(
		info.Identity, atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig,
	)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		return nil, fmt.Errorf("create query auth manager: %w", err)
	}

	// Store AuthManager in info so downstream operations can reuse it.
	if authManager != nil {
		info.AuthManager = authManager
		log.Debug("Created AuthManager for multi-component execution")
	}

	return authManager, nil
}

// executeTerraformQueryComponent runs one scheduled Terraform component and captures optional output.
// Per-node lifecycle hooks (user + CI, before and after) are handled one layer up by
// TerraformDispatcher.Dispatch via info.NodeHooks — this function stays hook-unaware.
func executeTerraformQueryComponent(execution scheduleradapters.TerraformExecution) (scheduleradapters.TerraformExecutionResult, error) {
	info := execution.Info
	opts := []ShellCommandOption{WithProcessContext(execution.Context)}
	if execution.Stdout != nil || execution.Stderr != nil {
		opts = append(opts, WithProcessStreams(process.Streams{
			Stdin:  os.Stdin,
			Stdout: execution.Stdout,
			Stderr: execution.Stderr,
		}))
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if execution.CaptureOutput {
		opts = append(opts, WithStdoutCapture(&stdoutBuf), WithStderrCapture(&stderrBuf))
	}

	execErr := ExecuteTerraform(info, opts...)
	return scheduleradapters.TerraformExecutionResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}, execErr
}
