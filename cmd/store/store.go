// Package store implements the `atmos store` command group for raw store CRUD.
package store

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

// storeParser holds the persistent flags shared by all store subcommands.
var storeParser *flags.StandardParser

// storeCmd is the parent command for raw store management.
var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage raw values in configured store backends.",
	Long: "Read, write, delete, and list values in the store backends configured under `stores:` " +
		"in atmos.yaml (AWS SSM, AWS Secrets Manager, HashiCorp Vault, Azure Key Vault, GCP Secret " +
		"Manager, Redis, Artifactory, 1Password, Keychain, GitHub Actions). Unlike `atmos secret`, " +
		"store values need no declaration: any key can be read, written, or deleted directly by " +
		"name against a named store. Values written here are readable via the !store/!store.get " +
		"YAML functions.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	defer perf.Track(nil, "store.init")()

	storeParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithStringFlag("component", "c", "", "Atmos component"),
		flags.WithEnvVars("component", "ATMOS_COMPONENT"),
		flags.WithStringFlag(cfg.IdentityFlagName, "i", "", "Identity to use when accessing the store backend"),
		flags.WithEnvVars(cfg.IdentityFlagName, "ATMOS_IDENTITY"),
	)

	storeParser.RegisterPersistentFlags(storeCmd)
	if err := storeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	storeCmd.AddCommand(setCmd)
	storeCmd.AddCommand(getCmd)
	storeCmd.AddCommand(deleteCmd)
	storeCmd.AddCommand(listCmd)
	storeCmd.AddCommand(keysCmd)

	internal.Register(&StoreCommandProvider{})
}

// StoreCommandProvider implements the CommandProvider interface.
type StoreCommandProvider struct{}

// GetCommand returns the store command.
func (p *StoreCommandProvider) GetCommand() *cobra.Command { return storeCmd }

// GetName returns the command name.
func (p *StoreCommandProvider) GetName() string { return "store" }

// GetGroup returns the command group for help organization.
func (p *StoreCommandProvider) GetGroup() string { return "Configuration" }

// GetFlagsBuilder returns the flags builder for this command.
func (p *StoreCommandProvider) GetFlagsBuilder() flags.Builder { return storeParser }

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (p *StoreCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags returns compatibility flags for this command.
func (p *StoreCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
func (p *StoreCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental returns whether this command is experimental.
func (p *StoreCommandProvider) IsExperimental() bool { return true }
