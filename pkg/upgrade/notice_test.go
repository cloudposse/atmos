package upgrade

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/installer"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

type testStreams struct{ stdout, stderr bytes.Buffer }

func (s *testStreams) Input() io.Reader     { return strings.NewReader("") }
func (s *testStreams) Output() io.Writer    { return &s.stdout }
func (s *testStreams) Error() io.Writer     { return &s.stderr }
func (s *testStreams) RawOutput() io.Writer { return &s.stdout }
func (s *testStreams) RawError() io.Writer  { return &s.stderr }

func TestPrintNotice(t *testing.T) {
	// UI formatter is process-global, so these cases intentionally run sequentially.
	t.Setenv("NO_COLOR", "1")
	for _, tt := range []struct {
		name         string
		installation installer.Installation
		command      string
	}{
		{"homebrew", installer.Installation{Kind: installer.Homebrew, Manager: "brew"}, "brew upgrade atmos"},
		{"deb direct or repository", installer.Installation{Kind: installer.DEB, Manager: "apt-get"}, "sudo apt-get update"},
		{"rpm direct or repository", installer.Installation{Kind: installer.RPM, Manager: "dnf"}, "sudo dnf upgrade atmos"},
		{"apk direct or repository", installer.Installation{Kind: installer.APK, Manager: "apk"}, "apk upgrade atmos"},
		{"aqua config first", installer.Installation{Kind: installer.Aqua, Manager: "aqua"}, "aqua install"},
		{"unknown", installer.Installation{Kind: installer.Unknown}, ""},
		{"unavailable", installer.Installation{Kind: installer.Homebrew}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			streams := &testStreams{}
			ctx, err := iolib.NewContext(iolib.WithStreams(streams))
			require.NoError(t, err)
			ui.InitFormatter(ctx)
			t.Cleanup(ui.Reset)
			hint := tt.installation.UpgradeHint("1.2.3")
			PrintNotice("1.2.2", "1.2.3", hint)
			assert.Empty(t, streams.stdout.String())
			output := streams.stderr.String()
			golden, err := os.ReadFile(filepath.Join("testdata", strings.ReplaceAll(tt.name, " ", "_")+".stderr.golden"))
			require.NoError(t, err)
			assert.Equal(t, string(golden), output)
			assert.Contains(t, output, "Update available! 1.2.2 » 1.2.3")
			assert.Contains(t, output, hint.URL)
			if tt.command == "" {
				assert.NotContains(t, output, "Run:")
			} else {
				assert.Contains(t, output, tt.command)
			}
			if hint.Condition != "" {
				require.Contains(t, output, hint.Condition)
				assert.Less(t, strings.Index(output, hint.Condition), strings.Index(output, "Run:"))
			}
			if hint.Message != "" {
				assert.Contains(t, output, hint.Message)
			}
		})
	}
}
