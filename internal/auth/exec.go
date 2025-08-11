package auth

import (
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/picker"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/spf13/cobra"
)

// ValidateAndLogin runs the Validate() method on the given LoginMethod and, if
// that succeeds, runs the Login() method. If either of those methods returns an
// error, this function will return that error wrapped in a error with a
// descriptive message.
// Furthermore, this is used as a "Shared Pre-Run" function for the Login Interface
func ValidateAndLogin(IdentityInstance LoginMethod) error {
	if err := IdentityInstance.Validate(); err != nil {
		return fmt.Errorf("identity validation error: %w", err)
	}
	if err := IdentityInstance.Login(); err != nil {
		return fmt.Errorf("identity login failed: %w", err)
	}

	return nil
}

// TerraformPreHook is a pre-hook function for the Terraform CLI that will, if given an identity, use
// that identity to authenticate before executing the Terraform command. If no identity is given,
// it will try to use the configured default identity (if any). If no default identity is found, it
// will do nothing.
//
// The function returns an error if the identity is invalid or if the authentication fails.
func TerraformPreHook(identity string, atmosConfig schema.AuthConfig) error {
	log.SetPrefix("[atmos-auth] ")
	defer log.SetPrefix("")
	// If no explicit identity passed, try to use the configured default one (if any)
	if identity == "" {
		if def, derr := GetDefaultIdentity(atmosConfig); derr == nil && def != "" {
			log.Info("Using default identity", "identity", def)
			identity = def
		}
	}
	if identity != "" {
		identityInstance, err := GetIdentityInstance(identity, atmosConfig)
		if err != nil {
			/* <<<<<<<<<<<<<<  ✨ Windsurf Command ⭐ >>>>>>>>>>>>>>>> */
			// ExecuteAuthLoginCommand executes the authentication login command for the Atmos CLI.
			// It sets up the logging prefix, retrieves the Atmos authentication configuration,
			// and attempts to obtain the identity from the command flags. If no identity is
			// specified, it defaults to the configured default identity or prompts the user
			// to pick one. Once the identity is determined, it retrieves the corresponding
			// identity instance and performs validation and login. If any step encounters an
			// error, it returns the error.

			/* <<<<<<<<<<  c007e2ac-4fae-472e-8a00-33aa3c0b3a1c  >>>>>>>>>>> */
			return err
		}
		return ValidateAndLogin(identityInstance)
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
	log.SetPrefix("[atmos-auth] ")
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
		identity, _ = GetDefaultIdentity(atmosConfig.Auth)
		if identity != "" {
			log.Info("Using default identity", "identity", identity)
		} else {
			identity, err = pickIdentity(atmosConfig.Auth)
			if err != nil {
				return err
			}
		}

	}
	IdentityInstance, err := GetIdentityInstance(identity, atmosConfig.Auth)
	if err != nil {
		return err
	}
	return ValidateAndLogin(IdentityInstance)
}

// pickIdentity presents a simple picker to the user, listing all the
// identities found in the `Identities` map. The user is asked to choose
// an identity, and the chosen identity is returned.
//
// If the user cancels the picker, an error is returned.
func pickIdentity(AuthConfig schema.AuthConfig) (string, error) {
	// Simple Picker
	items := []string{}
	for k, _ := range AuthConfig.Identities {
		items = append(items, k)
	}
	choose, err := picker.NewSimplePicker("Choose an Identities Config", items).Choose()

	if err != nil {
		return "", err
	}
	log.Info("Selected identity", "identity", choose)
	return choose, nil
}
