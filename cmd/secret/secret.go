// Package secret implements the `atmos secret` command group for declarative secrets CRUD.
package secret

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

// secretParser holds the persistent flags shared by all secret subcommands.
var secretParser *flags.StandardParser

// secretCmd is the parent command for declarative secrets management.
var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage declarative secrets across stacks and components.",
	Long: "Declare, provision, and resolve secrets backed by cloud secret stores (AWS SSM, " +
		"AWS Secrets Manager, HashiCorp Vault, Azure Key Vault, GCP Secret Manager, 1Password) or " +
		"SOPS-encrypted files. Secrets must be declared under a component's secrets.vars before they " +
		"can be set, read, or resolved via the !secret YAML function.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	defer perf.Track(nil, "secret.init")()

	secretParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithStringFlag("component", "c", "", "Atmos component"),
		flags.WithEnvVars("component", "ATMOS_COMPONENT"),
		flags.WithStringFlag("type", "", "", "Component type (terraform, helmfile, packer, ansible) to disambiguate"),
		flags.WithStringFlag(cfg.IdentityFlagName, "i", "", "Identity to use when accessing the secret backend"),
		flags.WithEnvVars(cfg.IdentityFlagName, "ATMOS_IDENTITY"),
	)

	secretParser.RegisterPersistentFlags(secretCmd)
	if err := secretParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	secretCmd.AddCommand(setCmd)
	secretCmd.AddCommand(getCmd)
	secretCmd.AddCommand(deleteCmd)
	secretCmd.AddCommand(listCmd)
	secretCmd.AddCommand(validateCmd)
	secretCmd.AddCommand(initCmd)
	secretCmd.AddCommand(pullCmd)
	secretCmd.AddCommand(pushCmd)
	secretCmd.AddCommand(importCmd)
	secretCmd.AddCommand(execCmd)
	secretCmd.AddCommand(shellCmd)

	internal.Register(&SecretCommandProvider{})
}

// SecretCommandProvider implements the CommandProvider interface.
type SecretCommandProvider struct{}

// GetCommand returns the secret command.
func (p *SecretCommandProvider) GetCommand() *cobra.Command { return secretCmd }

// GetName returns the command name.
func (p *SecretCommandProvider) GetName() string { return "secret" }

// GetGroup returns the command group for help organization.
func (p *SecretCommandProvider) GetGroup() string { return "Configuration" }

// GetFlagsBuilder returns the flags builder for this command.
func (p *SecretCommandProvider) GetFlagsBuilder() flags.Builder { return secretParser }

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (p *SecretCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags returns compatibility flags for this command.
func (p *SecretCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
func (p *SecretCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental returns whether this command is experimental.
func (p *SecretCommandProvider) IsExperimental() bool { return true }
