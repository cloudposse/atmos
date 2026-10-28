package exec

import (
	"io"
	"sync"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// newTestCommandWithGlobalFlags creates a test command with all global flags registered
// using the flag registry pattern. This ensures test commands have the same flags as
// production commands that inherit from RootCmd.
func newTestCommandWithGlobalFlags(use string) *cobra.Command {
	cmd := &cobra.Command{Use: use}

	// Register global flags using the same pattern as cmd/root.go.
	globalParser := flags.NewGlobalOptionsBuilder().Build()
	globalParser.RegisterPersistentFlags(cmd)

	return cmd
}

type synchronizedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *synchronizedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
