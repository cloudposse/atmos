package hooks

import "github.com/spf13/cobra"

// Command is the interface for all commands that can be run by hooks.
type Command interface {
	GetName() string
	RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error
}
