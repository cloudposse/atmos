package cast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRecordedCommandArgsOmitsCastFlag(t *testing.T) {
	got := recordedCommandArgs([]string{"--cast=/tmp/demo.cast", "terraform", "plan", "--stack", "dev"})
	want := []string{"terraform", "plan", "--stack", "dev"}
	if len(got) != len(want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
}

func TestStartRecordingSkipsHelpCompletionAndDisabled(t *testing.T) {
	activeCast = nil
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	helpCmd := newRecordingTestCommand("help")
	if err := StartRecordingIfRequested(helpCmd, &schema.AtmosConfiguration{}, []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("help command should not start cast recording")
	}

	disabledCmd := newRecordingTestCommand("version")
	if err := StartRecordingIfRequested(disabledCmd, &schema.AtmosConfiguration{}, []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("disabled recording should not start cast recording")
	}
}

func TestStartRecordingWithExplicitPath(t *testing.T) {
	activeCast = nil
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("terraform")
	if err := cmd.Flags().Set(FlagName, castPath); err != nil {
		t.Fatal(err)
	}

	err := StartRecordingIfRequested(
		cmd,
		&schema.AtmosConfiguration{
			Cast: schema.CastConfig{Recording: schema.CastRecordingConfig{Width: 100, Height: 30, Input: true}},
		},
		[]string{"--cast=" + castPath, "terraform", "plan", "--stack", "dev"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected active cast recording")
	}
	FinalizeRecording()

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatal(err)
	}
	headerLine := strings.SplitN(string(content), "\n", 2)[0]
	var header struct {
		Command string `json:"command"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
		t.Fatal(err)
	}
	if header.Command != "terraform plan --stack dev" {
		t.Fatalf("recorded command = %q", header.Command)
	}
	if header.Width != 100 || header.Height != 30 {
		t.Fatalf("size = %dx%d", header.Width, header.Height)
	}
}

func TestStartRecordingWithConfigEnabledUsesBasePath(t *testing.T) {
	activeCast = nil
	basePath := t.TempDir()
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	err := StartRecordingIfRequested(
		newRecordingTestCommand("workflow"),
		&schema.AtmosConfiguration{
			Cast: schema.CastConfig{Recording: schema.CastRecordingConfig{Enabled: true, BasePath: basePath}},
		},
		[]string{"workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected active cast recording")
	}
	path := activeCast.recorder.Path()
	if !strings.HasPrefix(path, basePath) {
		t.Fatalf("cast path = %q, want under %q", path, basePath)
	}
	if explicitPathValue("demo.cast", false) != "" {
		t.Fatal("non-explicit path should be empty")
	}
	if explicitPathValue("demo.cast", true) != "demo.cast" {
		t.Fatal("explicit path not preserved")
	}
	if len(environment()) == 0 {
		t.Fatal("expected process environment to be captured")
	}
	FinalizeRecording()
}

func newRecordingTestCommand(name string) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.Flags().String(FlagName, "", "")
	return cmd
}
