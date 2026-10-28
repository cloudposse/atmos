package version

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

var _ = schema.VersionCheck{Enabled: true, Frequency: "daily"}

type versionStreams struct{ stdout, stderr bytes.Buffer }

func (s *versionStreams) Input() io.Reader     { return &bytes.Buffer{} }
func (s *versionStreams) Output() io.Writer    { return &s.stdout }
func (s *versionStreams) Error() io.Writer     { return &s.stderr }
func (s *versionStreams) RawOutput() io.Writer { return &s.stdout }
func (s *versionStreams) RawError() io.Writer  { return &s.stderr }

func TestExecuteVersionCheckPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		ci, enabled, explicit, wantFetch bool
	}{
		{name: "CI automatic belongs to background notice", ci: true, enabled: true},
		{name: "CI explicit overrides disabled automatic checks", ci: true, explicit: true, wantFetch: true},
		{name: "local automatic remains enabled", enabled: true, wantFetch: true},
		{name: "local disabled remains disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserved := telemetry.PreserveCIEnvVars()
			t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
			if tc.ci {
				t.Setenv("CI", "true")
			}
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			t.Setenv("ATMOS_GITHUB_CLI", "")
			for _, key := range []string{"ATMOS_PRO_GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"} {
				t.Setenv(key, "")
			}
			mock := httpmock.NewGitHubMockServer(t)
			mock.Setenv(t)
			mock.RegisterRelease("cloudposse", "atmos", httpmock.ReleaseSpec{TagName: "v9999.0.0"})
			config := schema.AtmosConfiguration{}
			config.Version.Check = schema.VersionCheck{Enabled: tc.enabled, Frequency: "daily"}
			streams := &versionStreams{}
			ctx, err := iolib.NewContext(iolib.WithStreams(streams))
			require.NoError(t, err)
			data.InitWriter(ctx)
			require.NoError(t, Execute(&config, tc.explicit, "json"))
			assert.Equal(t, tc.enabled, config.Version.Check.Enabled, "command policy must not mutate shared configuration")
			var output map[string]any
			require.NoError(t, json.Unmarshal(streams.stdout.Bytes(), &output))
			if tc.wantFetch {
				require.Len(t, mock.Requests(), 1)
				assert.Equal(t, "9999.0.0", output["update_version"])
			} else {
				assert.Empty(t, mock.Requests())
				assert.NotContains(t, output, "update_version")
			}
			assert.Empty(t, streams.stderr.String())
		})
	}
}
