package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// registerCustomCommandIdentity retains the shared identity behavior without
// claiming a shorthand already owned by an inherited flag, such as Ansible's
// inventory -i. Copy the definition so unrelated commands still retain -i.
func registerCustomCommandIdentity(command, parent *cobra.Command) {
	parser := flags.NewStandardParser(flags.WithIdentityFlag())
	identity, ok := parser.Registry().Get(customCommandKeyIdentity).(*flags.StringFlag)
	if !ok || identity.Shorthand == "" {
		parser.RegisterPersistentFlags(command)
		return
	}
	inherited := parent.PersistentFlags().ShorthandLookup(identity.Shorthand)
	if inherited == nil {
		inherited = parent.InheritedFlags().ShorthandLookup(identity.Shorthand)
	}
	if inherited != nil && inherited.Name != customCommandKeyIdentity {
		definition := *identity
		definition.Shorthand = ""
		registry := flags.NewFlagRegistry()
		registry.Register(&definition)
		parser = flags.NewStandardParser(flags.WithRegistry(registry))
	}
	parser.RegisterPersistentFlags(command)
}
