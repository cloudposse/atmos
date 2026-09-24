package toolchain

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBatchInstallDeclarationPolicy exercises declaration writes and preservation
// across fresh/cached tools and sequential/concurrent installation.
func TestBatchInstallDeclarationPolicy(t *testing.T) {
	setupTestIO(t)
	for _, concurrency := range []int{1, 2} {
		for _, mode := range []string{"explicit", "automatic", "manifest", "frozen"} {
			for _, cached := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/concurrency=%d/cached=%t", mode, concurrency, cached), func(t *testing.T) {
					config, manifest, specs := batchDeclarationFixture(t)
					if cached || mode == "frozen" {
						require.NoError(t, RunAutomaticInstallBatch(specs, false))
						if !cached {
							require.NoError(t, os.RemoveAll(filepath.Join(config.Toolchain.InstallPath, "bin")))
						}
					}
					content := "# Existing declaration.\nowner/existing 0.5.0\n"
					if mode == "manifest" {
						content = "# Keep comments and order.\nowner/second 1.0.0\nowner/first 1.0.0\n"
					}
					require.NoError(t, os.WriteFile(manifest, []byte(content), 0o644))
					config.Toolchain.FrozenLockFile = mode == "frozen"
					if mode == "manifest" {
						require.NoError(t, RunInstallFromToolVersions(false, false, concurrency))
					} else {
						require.NoError(t, RunInstallBatchWithOptions(specs, BatchInstallOptions{
							MaxConcurrency: concurrency, SkipToolVersionsUpdate: mode == "automatic",
						}))
					}
					if mode == "explicit" {
						versions, err := LoadToolVersions(manifest)
						require.NoError(t, err)
						require.Equal(t, map[string][]string{
							"owner/existing": {"0.5.0"}, "owner/first": {"1.0.0"}, "owner/second": {"1.0.0"},
						}, versions.Tools)
					} else {
						after, err := os.ReadFile(manifest)
						require.NoError(t, err)
						require.Equal(t, content, string(after))
					}
					entries, err := os.ReadDir(".")
					require.NoError(t, err)
					require.Empty(t, entries, "declarations must use the configured project path")
				})
			}
		}
	}
}

func TestExplicitInstallPreservesAliasDeclarations(t *testing.T) {
	setupTestIO(t)
	for _, mode := range []string{"single", "sequential batch", "concurrent batch"} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cached=%t", mode, cached), func(t *testing.T) {
				config, manifest, specs := batchDeclarationFixture(t)
				config.Toolchain.Aliases = map[string]string{"first": "owner/first", "second": "owner/second"}
				if cached {
					require.NoError(t, RunAutomaticInstallBatch(specs, false))
				}
				require.NoError(t, os.WriteFile(manifest, []byte("first 0.5.0\nsecond 0.5.0\n"), 0o644))
				aliasSpecs := []string{"first@1.0.0", "second@1.0.0"}
				if mode == "single" {
					for _, spec := range aliasSpecs {
						require.NoError(t, RunInstallBatchWithOptions([]string{spec}, BatchInstallOptions{}))
					}
				} else {
					concurrency := 1
					if mode == "concurrent batch" {
						concurrency = 2
					}
					require.NoError(t, RunInstallBatchWithOptions(aliasSpecs, BatchInstallOptions{MaxConcurrency: concurrency}))
				}
				versions, err := LoadToolVersions(manifest)
				require.NoError(t, err)
				require.Equal(t, map[string][]string{
					"first": {"0.5.0", "1.0.0"}, "second": {"0.5.0", "1.0.0"},
				}, versions.Tools, "single and batch installs must append under the original aliases without changing defaults")
				installer := NewInstaller()
				for _, repo := range []string{"first", "second"} {
					binary, err := installer.FindBinaryPath("owner", repo, "1.0.0")
					require.NoError(t, err, "installation must still use canonical tool identities")
					require.FileExists(t, binary)
				}
			})
		}
	}
}

// batchDeclarationFixture serves two tools without relying on public registries.
func batchDeclarationFixture(t *testing.T) (*schema.AtmosConfiguration, string, []string) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("test executable"))
	}))
	t.Cleanup(server.Close)
	project := t.TempDir()
	config := &schema.AtmosConfiguration{
		BasePathAbsolute: project,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(project, "tools"), UseLockFile: true,
			Registries: []schema.ToolchainRegistry{{Name: "local", Type: "atmos", Tools: map[string]any{
				"owner/first":  map[string]any{"type": "http", "url": server.URL + "/first", "format": "raw"},
				"owner/second": map[string]any{"type": "http", "url": server.URL + "/second", "format": "raw"},
			}}},
		},
	}
	previous := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(previous) })
	SetAtmosConfig(config)
	return config, filepath.Join(project, ".tool-versions"), []string{"owner/first@1.0.0", "owner/second@1.0.0"}
}
