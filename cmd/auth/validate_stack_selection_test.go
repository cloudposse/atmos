package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthValidateSelectsPhysicalStackBeforeLoadingImports(t *testing.T) {
	for _, selected := range []string{"dev", "remote"} {
		t.Run(selected, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte("vars: {}\n"))
			}))
			t.Cleanup(server.Close)
			configPath := filepath.Join(root, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			config = []byte(strings.ReplaceAll(string(config), "included_paths: ['deploy/**/*']", "included_paths: ['dev', 'remote']"))
			require.NoError(t, os.WriteFile(configPath, config, 0o600))
			local, err := os.ReadFile(filepath.Join(root, "stacks", "deploy", "dev.yaml"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), local, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "remote.yaml"), []byte("import: ["+server.URL+"/stack.yaml]\n"), 0o600))
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--stack", selected}))
			err = executeAuthValidateCommand(cmd, nil)
			if selected == "dev" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "without fetching it")
			}
			require.Zero(t, requests.Load())
		})
	}
}
