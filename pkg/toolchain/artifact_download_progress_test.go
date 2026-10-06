package toolchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestArtifactDownloadProgressBar(t *testing.T) {
	for _, tt := range []struct {
		name       string
		tty        bool
		width      string
		downloaded int64
		total      int64
		filled     int
		empty      int
	}{
		{name: "start", tty: true, width: "120", total: 100, empty: 24},
		{name: "half", tty: true, width: "120", downloaded: 50, total: 100, filled: 12, empty: 12},
		{name: "complete", tty: true, width: "120", downloaded: 100, total: 100, filled: 24},
		{name: "over total", tty: true, width: "120", downloaded: 110, total: 100, filled: 24},
		{name: "narrow", tty: true, width: "50", downloaded: 50, total: 100, filled: 4, empty: 4},
		{name: "too narrow", tty: true, width: "40", downloaded: 50, total: 100},
		{name: "unknown total", tty: true, width: "120", downloaded: 50},
		{name: "logs", width: "120", downloaded: 50, total: 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", tt.width)
			t.Setenv("ATMOS_CAST_RECORDING_WIDTH", "")
			captureUITestOutput(t, func() {
				viper.Set("force-tty", tt.tty)
				message := formatArtifactDownloadProgress("artifact", tt.downloaded, tt.total)
				// The spinner renders messages as inline Markdown; the bar must survive it.
				text := ansi.Strip(ui.FormatInline(message))
				assert.Equal(t, tt.filled, strings.Count(text, "█"))
				assert.Equal(t, tt.empty, strings.Count(text, "░"))
				assert.Contains(t, text, "Downloading artifact")
				assert.NotContains(t, text, "\n")
			})
		})
	}
}

func TestDownloadPRArtifact_Progress(t *testing.T) {
	for _, tt := range []struct {
		name          string
		contentLength int64
		artifactSize  int64
		wantTotal     int64
	}{
		{name: "prefer HTTP length", contentLength: 3, artifactSize: 100, wantTotal: 3},
		{name: "fallback to artifact size", contentLength: -1, artifactSize: 3, wantTotal: 3},
		{name: "unknown length", contentLength: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := http.DefaultTransport
			http.DefaultTransport = artifactTestTransport{
				contentLength: tt.contentLength,
				readDelays:    []time.Duration{750 * time.Millisecond, 750 * time.Millisecond, 750 * time.Millisecond},
			}
			t.Cleanup(func() { http.DefaultTransport = original })
			synctest.Test(t, func(t *testing.T) {
				var received []int64
				path, err := downloadPRArtifactWithOptions(context.Background(), "", &github.PRArtifactInfo{
					DownloadURL: "https://example.com/artifact.zip",
					SizeInBytes: tt.artifactSize,
				}, artifactDownloadOptions{
					idleTimeout: time.Second,
					progress: func(downloaded, total int64) {
						received = append(received, downloaded)
						assert.Equal(t, tt.wantTotal, total)
					},
				})
				require.NoError(t, err)
				defer os.Remove(path)
				assert.Equal(t, []int64{0, 1, 2, 3, 3}, received, "report bytes during the transfer, not just at completion")
			})
		})
	}
}

func TestArtifactProgressReporter_ThrottlesOutput(t *testing.T) {
	// captureUITestOutput disables TTY mode, matching CI/log output.
	captureUITestOutput(t, func() {
		synctest.Test(t, func(t *testing.T) {
			var messages []string
			report := newArtifactProgressReporter("artifact", func(message string) { messages = append(messages, message) })
			report(0, 100)
			report(10, 100)
			time.Sleep(5 * time.Second)
			report(50, 100)
			report(100, 100)
			report(100, 100)
			assert.Equal(t, []string{
				"Downloading artifact (0b / 100b, 0%)",
				"Downloading artifact (50b / 100b, 50%)",
				"Downloading artifact (100b / 100b, 100%)",
			}, messages)
			assert.Equal(t, "Downloading artifact (50b received)", formatArtifactDownloadProgress("artifact", 50, 0))
		})
	})
}

func TestDownloadAndInstallArtifactToDir_ProgressAndSilentMode(t *testing.T) {
	for _, tt := range []struct {
		name         string
		showProgress bool
		color        bool
	}{
		{name: "progress/plain", showProgress: true},
		{name: "progress/color", showProgress: true, color: true},
		{name: "silent/plain"},
		{name: "silent/color", color: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLICOLOR_FORCE", "1")
			if tt.color {
				t.Setenv("NO_COLOR", "")
			} else {
				t.Setenv("NO_COLOR", "1")
			}
			tempDir := t.TempDir()
			cleanup := setupTestInstallPath(t, tempDir)
			defer cleanup()
			binaryName := "atmos"
			if runtime.GOOS == "windows" {
				binaryName += ".exe"
			}
			zipPath := filepath.Join(tempDir, "artifact.zip")
			createTestZip(t, zipPath, map[string][]byte{binaryName: []byte("test binary")})
			zipData, err := os.ReadFile(zipPath)
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(zipData)
			}))
			defer server.Close()
			output := captureUITestOutput(t, func() {
				path, err := downloadAndInstallArtifactToDir(context.Background(), "", "pr-test", &github.PRArtifactInfo{
					DownloadURL: server.URL, ArtifactName: "build-artifacts-test", SizeInBytes: int64(len(zipData)),
				}, tt.showProgress)
				require.NoError(t, err)
				binary, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "test binary", string(binary))
			})
			if tt.showProgress {
				if tt.color {
					assert.Contains(t, output, "\x1b[", "exercise ANSI output even outside CI")
				}
				// Styling can insert escape sequences between words in the message.
				text := ansi.Strip(output)
				assert.Contains(t, text, "Downloading build-artifacts-test")
				assert.Contains(t, text, "100%")
				assert.Contains(t, text, "Extracting build-artifacts-test")
				assert.Contains(t, text, "Installed to")
			} else {
				assert.Empty(t, output)
			}
		})
	}
}
