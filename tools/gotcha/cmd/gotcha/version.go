package main

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

// Version information set at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// newVersionCmd creates the version subcommand.
func newVersionCmd(logger *log.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gotcha version %s\n", Version)
			fmt.Printf("  Build time: %s\n", BuildTime)
			fmt.Printf("  Git commit: %s\n", GitCommit)
		},
	}
}
