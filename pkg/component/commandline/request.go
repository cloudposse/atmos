package commandline

import "github.com/spf13/cobra"

// Request contains the component command and arguments to parse.
type Request struct {
	ComponentType          string
	Command                *cobra.Command
	Args                   []string
	AdditionalArgsAndFlags []string
}
