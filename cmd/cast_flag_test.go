package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCastFlagDoesNotConsumeNextArgument(t *testing.T) {
	flag := RootCmd.PersistentFlags().Lookup("cast")
	if flag == nil {
		t.Fatal("cast flag is not registered")
	}
	if flag.NoOptDefVal == "" {
		t.Fatal("cast flag must support bare --cast")
	}

	args := []string{"--cast", "terraform", "plan"}
	parsed := preprocessNoOptDefValFlags(args)
	if len(parsed) != len(args) {
		t.Fatalf("cast flag preprocessing consumed an argument: got %#v", parsed)
	}
	for i := range args {
		if parsed[i] != args[i] {
			t.Fatalf("unexpected preprocessing result: got %#v want %#v", parsed, args)
		}
	}
}

func TestCastRecordedCommandArgsOmitsCastFlag(t *testing.T) {
	got := castRecordedCommandArgs([]string{"--cast=/tmp/demo.cast", "terraform", "plan", "--stack", "dev"})
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

func TestStartCastRecordingSkipsHelpCompletionAndDisabled(t *testing.T) {
	activeCast = nil
	t.Cleanup(func() {
		if activeCast != nil {
			finalizeCastRecording()
		}
	})

	helpCmd := newCastTestCommand("help")
	if err := startCastRecordingIfRequested(helpCmd, &schema.AtmosConfiguration{}); err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("help command should not start cast recording")
	}

	disabledCmd := newCastTestCommand("version")
	if err := startCastRecordingIfRequested(disabledCmd, &schema.AtmosConfiguration{}); err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("disabled recording should not start cast recording")
	}
}

func TestStartCastRecordingWithExplicitPath(t *testing.T) {
	activeCast = nil
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	oldArgs := os.Args
	os.Args = []string{"atmos", "--cast=" + castPath, "terraform", "plan", "--stack", "dev"}
	t.Cleanup(func() {
		os.Args = oldArgs
		if activeCast != nil {
			finalizeCastRecording()
		}
	})

	cmd := newCastTestCommand("terraform")
	if err := cmd.Flags().Set(castFlagName, castPath); err != nil {
		t.Fatal(err)
	}

	err := startCastRecordingIfRequested(cmd, &schema.AtmosConfiguration{
		Cast: schema.CastConfig{Recording: schema.CastRecordingConfig{Width: 100, Height: 30, Input: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected active cast recording")
	}
	finalizeCastRecording()

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

func TestStartCastRecordingWithConfigEnabledUsesBasePath(t *testing.T) {
	activeCast = nil
	basePath := t.TempDir()
	oldArgs := os.Args
	os.Args = []string{"atmos", "workflow", "demo"}
	t.Cleanup(func() {
		os.Args = oldArgs
		if activeCast != nil {
			finalizeCastRecording()
		}
	})

	err := startCastRecordingIfRequested(newCastTestCommand("workflow"), &schema.AtmosConfiguration{
		Cast: schema.CastConfig{Recording: schema.CastRecordingConfig{Enabled: true, BasePath: basePath}},
	})
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
	if len(castEnvironment()) == 0 {
		t.Fatal("expected process environment to be captured")
	}
	finalizeCastRecording()
}

func newCastTestCommand(name string) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.Flags().String(castFlagName, "", "")
	return cmd
}
