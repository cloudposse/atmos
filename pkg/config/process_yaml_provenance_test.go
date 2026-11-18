package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
)

// TestLoadConfig_ArtifactsNestedIncludeKeepsRemoteProvenance covers wrapped includes: the
// innermost remote origin survives an outer local wrapper, and a remote wrapper stays remote.
func TestLoadConfig_ArtifactsNestedIncludeKeepsRemoteProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/keys.yaml":
			_, _ = fmt.Fprint(w, "- release.pub\n")
		case "/key.yaml":
			_, _ = fmt.Fprint(w, "release.pub\n")
		case "/wrap-local.yaml":
			_, _ = fmt.Fprint(w, "!include ./local-keys.yaml | eval\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name       string
		wrapper    string
		wantRemote bool
		// section replaces the default `trusted_keys: !include ./trust/wrap.yaml` line when set.
		section string
		// plain reports that the wrapper holds no tags, so it also loads without `| eval`.
		plain bool
	}{
		{name: "local wrapper around a remote include", wrapper: "!include " + server.URL + "/keys.yaml | eval\n", wantRemote: true},
		{name: "local wrapper around a sequence holding a remote include", wrapper: "- !include " + server.URL + "/key.yaml | eval\n", wantRemote: true},
		{name: "remote wrapper around a local include", wrapper: "!include " + server.URL + "/wrap-local.yaml | eval\n", wantRemote: true},
		{
			name:       "local mapping wrapper around a remote key list",
			wrapper:    "trusted_keys: !include " + server.URL + "/keys.yaml | eval\n",
			section:    "verification: !include ./trust/wrap.yaml | eval\n",
			wantRemote: true,
		},
		{name: "local wrapper around local content stays local", wrapper: "- release.pub\n", wantRemote: false, plain: true},
	}
	for _, tt := range tests {
		suffixes := []string{" | eval"}
		if tt.plain {
			suffixes = append(suffixes, "")
		}
		for _, suffix := range suffixes {
			t.Run(fmt.Sprintf("%s/eval=%t", tt.name, suffix != ""), func(t *testing.T) {
				section := "  verification:\n    trusted_keys: !include ./trust/wrap.yaml" + suffix + "\n"
				if tt.section != "" {
					section = "  " + tt.section
				}
				dir := artifactsTestDir(t, "artifacts:\n"+section)
				writeArtifactsFile(t, filepath.Join(dir, "trust", "wrap.yaml"), tt.wrapper)
				writeArtifactsFile(t, filepath.Join(dir, "trust", "local-keys.yaml"), "- release.pub\n")
				cfg, err := loadArtifactsDir(t, dir)
				require.NoError(t, err)
				require.NotNil(t, cfg.Artifacts.Verification)
				assert.Equal(t, tt.wantRemote, cfg.Artifacts.Verification.KeysOrigin.Remote)
				if !tt.wantRemote {
					assert.Equal(t, []string{filepath.Join(dir, "trust", "release.pub")}, cfg.Artifacts.Verification.TrustedKeys)
					return
				}
				assert.Equal(t, []string{"release.pub"}, cfg.Artifacts.Verification.TrustedKeys, "a relative key from remote content is never anchored locally")
				err = artifactconfig.Validate(&cfg.Artifacts)
				require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
				assert.Contains(t, err.Error(), "remote import")
			})
		}
	}
}
