package auth

import (
	"fmt"
	"github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/spf13/cobra"
)

const (
	LogPrefix = "[atmos-auth]"
)

// ValidateLoginAssumeRole runs the Validate() method on the given LoginMethod and, if
// that succeeds, runs the Login() method. If either of those methods returns an
// error, this function will return that error wrapped in a error with a
// descriptive message.
// Furthermore, this is used as a "Shared Pre-Run" function for the Login Interface
func ValidateLoginAssumeRole(IdentityInstance LoginMethod, atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if err := IdentityInstance.Validate(); err != nil {
		return fmt.Errorf("identity validation error: %w", err)
	}
	if err := IdentityInstance.Login(); err != nil {
		return fmt.Errorf("identity login failed: %w", err)
	}
	if err := IdentityInstance.AssumeRole(); err != nil {
		return fmt.Errorf("identity assume role failed: %w", err)
	}
	if err := IdentityInstance.SetEnvVars(info); err != nil {
		return fmt.Errorf("identity set env vars failed: %w", err)
	}
	return nil
}

// TerraformPreHook is a pre-hook function for the Terraform CLI that will, if given an identity, use
// that identity to authenticate before executing the Terraform command. If no identity is given,
// it will try to use the configured default identity (if any). If no default identity is found, it
// will do nothing.
//
// The function returns an error if the identity is invalid or if the authentication fails.
func TerraformPreHook(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	authConfig := atmosConfig.Auth
	log.SetPrefix(LogPrefix)
	defer log.SetPrefix("")

	identity := info.Identity // Set by CLI Flags
	// If no explicit identity was passed, try to use the configured default one (if any)
	if identity == "" {
		def, derr := GetDefaultIdentity(info.ComponentIdentitiesSection)
		if derr == nil && def != "" {
			log.Info("Using default identity", "identity", def)
			identity = def
		}
		log.Debug("TerraformPreHook[GetDefaultIdentity]", "default", def, "identity", info.Identity, "derr", derr)

	}
	// If we don't have a default, but several are enabled, prompt the user, if not in CI
	if telemetry.IsCI() {
		return nil
	}
	if identity == "" {
		identity, _ = pickIdentity(GetEnabledIdentities(info.ComponentIdentitiesSection))
	}

	if identity != "" {
		identityInstance, err := GetIdentityInstance(identity, authConfig, info)
		if err != nil {
			return err
		}
		err = ValidateLoginAssumeRole(identityInstance, atmosConfig, info)

		return err
	}

	return nil
}

// ExecuteAuthLoginCommand executes the authentication login command for the Atmos CLI.
//
// It sets up the logging prefix, retrieves the Atmos authentication configuration,
// and attempts to obtain the identity from the command flags. If no identity is
// specified, it defaults to the configured default identity or prompts the user
// to pick one. Once the identity is determined, it retrieves the corresponding
// identity instance and performs validation and login. If any step encounters an
// error, it returns the error.
func ExecuteAuthLoginCommand(cmd *cobra.Command, args []string) error {
	log.SetPrefix(LogPrefix)
	defer log.SetPrefix("")

	flags := cmd.Flags()

	// Get Atmos Auth Configuration
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}
	Identities := atmosConfig.Auth.Identities
	if Identities == nil {
		log.Fatal("no auth identities found")
	}

	// Get IdentityFlag or prompt for one
	identity, err := flags.GetString("identity")
	if err != nil {
		return err
	}
	if identity == "" {
		identity, _ = GetDefaultIdentity(atmosConfig.Auth.Identities)
		if identity != "" {
			log.Info("Using default identity", "identity", identity)
		} else {
			identity, err = pickKeyFromMap(atmosConfig.Auth.Identities)
			if err != nil {
				return err
			}
		}

	}
	IdentityInstance, err := GetIdentityInstance(identity, atmosConfig.Auth, nil)
	if err != nil {
		return err
	}
	return ValidateLoginAssumeRole(IdentityInstance, atmosConfig, nil)
}
