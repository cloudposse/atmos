package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authGitCredentialCmd implements Git credential helper protocol.
// This allows Atmos to be used as a Git credential helper:
//
//	git config --global credential.helper '!atmos auth git-credential'
//
// Inspired by ghtkn: https://github.com/suzuki-shunsuke/ghtkn
var authGitCredentialCmd = &cobra.Command{
	Use:   "git-credential",
	Short: "Git credential helper for GitHub authentication",
	Long: `Act as a Git credential helper for GitHub authentication.

Configure git to use Atmos for GitHub credentials:

  git config --global credential.helper '!atmos auth git-credential'

Or for specific repositories:

  git config credential.https://github.com.helper '!atmos auth git-credential'

This command implements the Git credential helper protocol and provides
GitHub tokens from Atmos authentication.`,
	Args:               cobra.ExactArgs(1),
	ValidArgs:          []string{"get", "store", "erase"},
	DisableFlagParsing: false,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeGitCredentialCommand,
}

// executeGitCredentialCommand implements the Git credential helper protocol.
func executeGitCredentialCommand(cmd *cobra.Command, args []string) error {
	operation := args[0]

	switch operation {
	case "get":
		return handleGitCredentialGet(cmd)
	case "store":
		// No-op: Atmos manages its own token storage
		return nil
	case "erase":
		// No-op: Use `atmos auth logout` to clear tokens
		return nil
	default:
		return fmt.Errorf("%w: unknown git credential operation: %s", errUtils.ErrInvalidSubcommand, operation)
	}
}

// handleGitCredentialGet handles the "get" operation of git credential protocol.
func handleGitCredentialGet(cmd *cobra.Command) error {
	// Read input from stdin (git sends key=value pairs).
	input, err := readGitCredentialInput()
	if err != nil {
		return fmt.Errorf("failed to read git credential input: %w", err)
	}

	// Only provide credentials for github.com.
	if !strings.Contains(input["host"], "github.com") {
		// Return empty response for non-GitHub hosts.
		return nil
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// Get identity from flag or use default.
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName == "" {
		defaultIdentity, err := authManager.GetDefaultIdentity()
		if err != nil {
			// No default identity, return empty (git will try other helpers).
			return nil
		}
		identityName = defaultIdentity
	}

	// Check if identity is a GitHub provider.
	providerKind, err := authManager.GetProviderKindForIdentity(identityName)
	if err != nil || (providerKind != "github/user" && providerKind != "github/app") {
		// Not a GitHub identity, return empty.
		return nil
	}

	// Authenticate and get credentials.
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Extract GitHub token from environment variables.
	token, ok := whoami.Environment["GITHUB_TOKEN"]
	if !ok || token == "" {
		return fmt.Errorf("%w: no GITHUB_TOKEN in authentication result", errUtils.ErrAuthenticationFailed)
	}

	// Output credentials in git credential helper format.
	fmt.Printf("username=x-access-token\n")
	fmt.Printf("password=%s\n", token)

	return nil
}

// readGitCredentialInput reads key=value pairs from stdin (git credential protocol).
func readGitCredentialInput() (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break // Empty line signals end of input.
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func init() {
	authGitCredentialCmd.Flags().StringP("identity", "i", "", "GitHub identity to use for credentials")
	AddIdentityCompletion(authGitCredentialCmd)
	authCmd.AddCommand(authGitCredentialCmd)
}
