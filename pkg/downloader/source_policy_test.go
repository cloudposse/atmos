package downloader_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/downloader"
	_ "github.com/cloudposse/atmos/pkg/policy/admission"
)

func TestDownloadSourcePolicyBeforeIOAndRedirect(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	var requests atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write([]byte("secret")) }))
	defer forbidden.Close()
	forbiddenURL := strings.Replace(forbidden.URL, "127.0.0.1", "localhost", 1)
	manifest := fmt.Sprintf("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: downloads}\nspec:\n  policies:\n    deny:\n      on: source\n      action: deny\n      rule: %q\n", "input.host == 'localhost'")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	fetch := downloader.NewGoGetterDownloader(nil)
	destination := filepath.Join(t.TempDir(), "out")
	require.Error(t, fetch.Fetch(forbiddenURL, destination, downloader.ClientModeFile, 5*time.Second))
	require.Zero(t, requests.Load())
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, forbiddenURL, http.StatusFound) }))
	defer redirect.Close()
	require.Error(t, fetch.Fetch(redirect.URL, destination, downloader.ClientModeFile, 5*time.Second))
	require.Zero(t, requests.Load())
	content, err := os.ReadFile(destination)
	if err == nil {
		require.Empty(t, content)
	} else {
		require.True(t, os.IsNotExist(err))
	}
}
