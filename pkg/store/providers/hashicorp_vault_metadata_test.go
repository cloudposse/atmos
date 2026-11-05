package providers

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// TestVaultStore_Has_CurrentVersion verifies availability without reading secret data.
func TestVaultStore_Has_CurrentVersion(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
	active := map[string]any{"deletion_time": "", "destroyed": false}
	deleted := map[string]any{"deletion_time": past, "destroyed": false}
	tests := []struct {
		name     string
		current  int
		versions map[string]any
		status   int
		want     bool
	}{
		{name: "readable latest", current: 2, versions: map[string]any{"1": active, "2": active}, want: true},
		{name: "deleted latest with readable older version", current: 2, versions: map[string]any{"1": active, "2": deleted}},
		{name: "readable latest with deleted older version", current: 2, versions: map[string]any{"1": deleted, "2": active}, want: true},
		{name: "destroyed latest", current: 2, versions: map[string]any{"2": map[string]any{"deletion_time": "", "destroyed": true}}},
		{name: "future scheduled deletion", current: 2, versions: map[string]any{"2": map[string]any{"deletion_time": future, "destroyed": false}}, want: true},
		{name: "metadata without any versions"},
		{name: "missing current version", current: 2, versions: map[string]any{"1": active}},
		{name: "missing path", status: http.StatusNotFound},
		{name: "permission denied", status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/secret/metadata/shared-account", r.URL.Path)
				if tt.status != 0 {
					writeVaultJSON(w, tt.status, map[string]any{"errors": []string{"unavailable"}})
					return
				}
				metadata := map[string]any{"current_version": tt.current}
				if tt.versions != nil {
					metadata["versions"] = tt.versions
				}
				writeVaultJSON(w, http.StatusOK, map[string]any{"data": metadata})
			}))
			t.Cleanup(srv.Close)
			t.Setenv("VAULT_ADDR", "")
			t.Setenv("VAULT_TOKEN", "")
			raw, err := NewVaultStore(&VaultStoreOptions{Address: srv.URL, Mount: "secret", Token: "synthetic-token"}, "")
			require.NoError(t, err)
			s, ok := raw.(store.StatusStore)
			require.True(t, ok)

			has, err := s.Has("", "", "shared-account")
			if tt.status == http.StatusForbidden {
				require.ErrorIs(t, err, store.ErrVaultRead)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, has)
			assert.EqualValues(t, 1, requests.Load(), "Has must make exactly one metadata request")
		})
	}
}
