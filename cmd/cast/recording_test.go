package cast

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
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
	if len(environment()) == 0 {
		t.Fatal("expected process environment to be captured")
	}
	FinalizeRecording()
}

func TestPlanRecordingOutputUsesExtension(t *testing.T) {
	if plan, err := planRecordingOutput("demo.cast", true); err != nil {
		t.Fatal(err)
	} else if plan.castPath != "demo.cast" || plan.renderOutput != "" || plan.removeCast {
		t.Fatalf("unexpected cast plan: %#v", plan)
	}

	gifPath := filepath.Join(t.TempDir(), "demo.gif")
	plan, err := planRecordingOutput(gifPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.castPath != "" || plan.castBasePath != os.TempDir() || plan.renderOutput != gifPath || !plan.removeCast || plan.explicitCast {
		t.Fatalf("unexpected gif plan: %#v", plan)
	}

	if _, err := planRecordingOutput(filepath.Join(t.TempDir(), "demo.txt"), true); !errors.Is(err, errUtils.ErrUnsupportedCastOutputExtension) {
		t.Fatalf("expected unsupported extension error, got %v", err)
	}
}

func TestPlanRecordingOutputRejectsExistingRenderedOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo.gif")
	if err := os.WriteFile(output, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := planRecordingOutput(output, true); !errors.Is(err, asciicast.ErrRenderOutputExists) {
		t.Fatalf("expected output exists error, got %v", err)
	}
}

func newRecordingTestCommand(name string) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.Flags().String(FlagName, "", "")
	return cmd
}
