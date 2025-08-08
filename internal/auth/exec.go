package auth

import (
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/picker"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/spf13/cobra"
)

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
			return err
		}
		if err = identityInstance.Validate(); err != nil {
			return fmt.Errorf("identity validation error: %w", err)
		}
		if err = identityInstance.Login(); err != nil {
			return fmt.Errorf("identity login failed: %w", err)
		}
	}

	return nil
}

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
	validationErr := IdentityInstance.Validate()
	if validationErr != nil {
		log.Fatal("IdentityFlag Validation Error", "error", validationErr, "config", IdentityInstance)
	}

	return IdentityInstance.Login()
	// Setup default region
	//identityConfig := Identities[identity]
	//if identityConfig.Region == "" {
	//	identityConfig.Region = atmosConfig.Auth.DefaultRegion
	//}
	//identityConfig.Alias = identity
	//
	//return auth.ExecuteAuth(identity, identityConfig)
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
