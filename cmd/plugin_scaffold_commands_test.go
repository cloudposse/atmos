package cmd

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// Exercise the actual generated custom commands through fresh CLI processes.
// The in-process registry keeps publication coverage independent of Docker.
func verifyGeneratedPluginPublish(t *testing.T, directory string, environment []string) {
	t.Helper()
	var requests, writes atomic.Int64
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writes.Add(1)
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)
	repository := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/example/oidc"
	// Temporary staging cleanup is verified directly by the release package tests;
	// subprocess coverage here checks that pushes never accumulate project layouts.
	assertCleanStaging := func() {
		t.Helper()
		require.NoDirExists(t, filepath.Join(directory, ".atmos", "release"), "source releases must not accumulate layouts in the project")
	}
	publishedRelease := func() *pluginoci.Release {
		t.Helper()
		reference, err := name.ParseReference(strings.TrimPrefix(repository, "oci://") + ":0.1.0")
		require.NoError(t, err)
		index, err := remote.Index(reference, remote.WithContext(t.Context()), remote.WithAuth(authn.Anonymous))
		require.NoError(t, err)
		release, err := pluginoci.Inspect(index, "")
		require.NoError(t, err)
		require.Equal(t, "example/oidc", release.Name)
		require.Equal(t, "0.1.0", release.Version)
		return release
	}
	run := func(arguments ...string) string { return runPluginCLI(t, directory, environment, arguments...) }
	help := run("push", "--help")
	for _, flag := range []string{"--to", "--sign", "--yes"} {
		require.Contains(t, help, flag)
	}
	require.NotContains(t, help, "--catalog")
	require.Contains(t, run("dev", "registry", "up", "--help"), "15557")
	run("validate", "stacks")
	output := run("push", "--to", repository)
	for _, message := range []string{"3 plugin checks passed", "Published"} {
		require.Contains(t, output, message)
	}
	first := publishedRelease()
	require.Contains(t, output, first.Digest)
	require.Positive(t, writes.Load(), "the first push must publish an actual OCI release")
	assertCleanStaging()
	// Releasing identical source is idempotent and leaves no staging layout behind.
	previousWrites := writes.Load()
	secondOutput := run("push", "--to", repository)
	second := publishedRelease()
	require.Equal(t, first.Digest, second.Digest)
	require.Contains(t, secondOutput, second.Digest)
	require.Equal(t, previousWrites, writes.Load(), "an identical immutable release must not be rewritten")
	assertCleanStaging()
	ignore, err := os.ReadFile(filepath.Join(directory, ".gitignore"))
	require.NoError(t, err)
	require.Contains(t, string(ignore), "/.atmos/")
	// Generated projects discover releases directly without publishing a catalog.
	run("plugin", "registry", "add", "local", "--host", strings.TrimPrefix(server.URL, "http://"), "--repository", repository)
	require.Contains(t, run("plugin", "search", "oidc", "--registry", "local"), "example/oidc")
	// A failed build must stop the native release before packaging or publication.
	source := filepath.Join(directory, "main.go")
	original, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte("package main\ninvalid Go source\n"), 0o600))
	t.Cleanup(func() { require.NoError(t, os.WriteFile(source, original, 0o600)) })
	executable, err := os.Executable()
	require.NoError(t, err)
	failed := exec.CommandContext(t.Context(), executable, "push", "--to", repository)
	failed.Dir, failed.Env = directory, environment
	previousRequests := requests.Load()
	failure, err := failed.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(failure), "syntax error")
	require.NotContains(t, string(failure), "Published")
	require.Equal(t, previousRequests, requests.Load(), "a failed build must never contact the destination registry")
	require.Equal(t, first.Digest, publishedRelease().Digest, "a failed build preserves the published release")
	assertCleanStaging()
	require.NoError(t, os.WriteFile(source, original, 0o600))
}
