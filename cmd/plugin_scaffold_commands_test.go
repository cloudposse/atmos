package cmd

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

// Exercise the actual generated custom commands through fresh CLI processes.
// The in-process registry keeps publication coverage independent of Docker.
func verifyGeneratedPluginPublish(t *testing.T, directory string, environment []string) {
	t.Helper()
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repository := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/example/oidc"
	run := func(arguments ...string) string { return runPluginCLI(t, directory, environment, arguments...) }
	help := run("push", "--help")
	for _, flag := range []string{"--to", "--sign", "--yes"} {
		require.Contains(t, help, flag)
	}
	require.NotContains(t, help, "--catalog")
	require.Contains(t, run("dev", "registry", "up", "--help"), "15557")
	run("validate", "stacks")
	output := run("push", "--to", repository)
	for _, message := range []string{"Plugin built successfully", "3 plugin checks passed", "Packed", "Published"} {
		require.Contains(t, output, message)
	}
	first, err := os.ReadFile(filepath.Join(directory, ".atmos", "release", "1", "index.json"))
	require.NoError(t, err)
	// Independent publication preserves layouts from previous pushes.
	run("push", "--to", repository)
	require.FileExists(t, filepath.Join(directory, ".atmos", "release", "2", "index.json"))
	preserved, err := os.ReadFile(filepath.Join(directory, ".atmos", "release", "1", "index.json"))
	require.NoError(t, err)
	require.Equal(t, first, preserved)
	ignore, err := os.ReadFile(filepath.Join(directory, ".gitignore"))
	require.NoError(t, err)
	require.Contains(t, string(ignore), "/.atmos/")
	// Generated projects discover releases directly without publishing a catalog.
	run("plugin", "registry", "add", "local", "--host", strings.TrimPrefix(server.URL, "http://"), "--repository", repository)
	require.Contains(t, run("plugin", "search", "oidc", "--registry", "local"), "example/oidc")
	// A failed build must stop the composed push before packaging or publication.
	source := filepath.Join(directory, "main.go")
	original, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte("package main\ninvalid Go source\n"), 0o600))
	t.Cleanup(func() { require.NoError(t, os.WriteFile(source, original, 0o600)) })
	executable, err := os.Executable()
	require.NoError(t, err)
	failed := exec.CommandContext(t.Context(), executable, "push", "--to", repository)
	failed.Dir, failed.Env = directory, environment
	failure, err := failed.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(failure), "syntax error")
	require.NotContains(t, string(failure), "Published")
	require.NoDirExists(t, filepath.Join(directory, ".atmos", "release", "3"))
	require.NoError(t, os.WriteFile(source, original, 0o600))
}
