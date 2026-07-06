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
	pkgFlags "github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRecordedCommandArgsCopiesResolvedArgs(t *testing.T) {
	input := []string{"terraform", "plan", "--stack", "dev"}
	got := recordedCommandArgs(input)
	want := []string{"terraform", "plan", "--stack", "dev"}
	if len(got) != len(want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
	got[0] = "changed"
	if input[0] != "terraform" {
		t.Fatalf("recordedCommandArgs must copy input, got input %#v", input)
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
			Cast: &schema.CastConfig{Recording: &schema.CastRecordingConfig{Width: 100, Height: 30, Input: true}},
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
		Term    struct {
			Cols int `json:"cols"`
			Rows int `json:"rows"`
		} `json:"term"`
	}
	if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
		t.Fatal(err)
	}
	if header.Command != "terraform plan --stack dev" {
		t.Fatalf("recorded command = %q", header.Command)
	}
	if header.Term.Cols != 100 || header.Term.Rows != 30 {
		t.Fatalf("size = %dx%d", header.Term.Cols, header.Term.Rows)
	}
}

func TestStartRecordingWithBareCastFlagUsesGeneratedPath(t *testing.T) {
	activeCast = nil
	cacheDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", cacheDir)
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("terraform")
	if err := cmd.Flags().Set(FlagName, autoFlagValue); err != nil {
		t.Fatal(err)
	}

	err := StartRecordingIfRequested(
		cmd,
		&schema.AtmosConfiguration{},
		[]string{"--cast", "terraform", "plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected bare --cast to start recording")
	}
	path := activeCast.recorder.Path()
	FinalizeRecording()
	if !strings.HasPrefix(path, filepath.Join(cacheDir, "atmos", "casts")) {
		t.Fatalf("cast path = %q, want generated path under cache", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	headerLine := strings.SplitN(string(content), "\n", 2)[0]
	var header struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
		t.Fatal(err)
	}
	if header.Command != "terraform plan" {
		t.Fatalf("recorded command = %q", header.Command)
	}
}

func TestResolveRecordingRequestWithDisableFlagParsingBareCast(t *testing.T) {
	cmd := newRecordingTestCommand("run")
	cmd.DisableFlagParsing = true

	request, err := resolveRecordingRequest(cmd, &schema.AtmosConfiguration{}, []string{"--cast", "terraform", "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if request.source != recordingSourceFlag || request.value != autoFlagValue {
		t.Fatalf("unexpected request: %#v", request)
	}
	wantArgs := []string{"terraform", "plan"}
	if strings.Join(request.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("request args = %#v, want %#v", request.args, wantArgs)
	}
}

func TestResolveRecordingRequestWithDisableFlagParsingCastPath(t *testing.T) {
	cmd := newRecordingTestCommand("run")
	cmd.DisableFlagParsing = true
	castPath := filepath.Join(t.TempDir(), "demo.cast")

	request, err := resolveRecordingRequest(cmd, &schema.AtmosConfiguration{}, []string{"--cast=" + castPath, "terraform", "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if request.source != recordingSourceFlag || request.value != castPath {
		t.Fatalf("unexpected request: %#v", request)
	}
	wantArgs := []string{"terraform", "plan"}
	if strings.Join(request.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("request args = %#v, want %#v", request.args, wantArgs)
	}
}

func TestActiveRecordingWidth(t *testing.T) {
	activeCast = nil
	if width := ActiveRecordingWidth(); width != 0 {
		t.Fatalf("ActiveRecordingWidth() = %d with no active recording, want 0", width)
	}

	castPath := filepath.Join(t.TempDir(), "width.cast")
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
			Cast: &schema.CastConfig{Recording: &schema.CastRecordingConfig{Width: 90, Height: 30}},
		},
		[]string{"--cast=" + castPath, "terraform", "plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if width := ActiveRecordingWidth(); width != 90 {
		t.Fatalf("ActiveRecordingWidth() = %d during recording, want 90", width)
	}
	FinalizeRecording()
	if width := ActiveRecordingWidth(); width != 0 {
		t.Fatalf("ActiveRecordingWidth() = %d after FinalizeRecording, want 0", width)
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
			Cast: &schema.CastConfig{Recording: &schema.CastRecordingConfig{Enabled: true, BasePath: basePath}},
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

func TestStartRecordingWithAtmosCastTrueUsesGeneratedPath(t *testing.T) {
	activeCast = nil
	cacheDir := t.TempDir()
	t.Setenv(EnvName, "true")
	t.Setenv("ATMOS_XDG_CACHE_HOME", cacheDir)
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	err := StartRecordingIfRequested(
		newRecordingTestCommand("workflow"),
		&schema.AtmosConfiguration{},
		[]string{"workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected ATMOS_CAST=true to start cast recording")
	}
	path := activeCast.recorder.Path()
	wantPrefix := filepath.Join(cacheDir, "atmos", "casts")
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("cast path = %q, want under %q", path, wantPrefix)
	}
	FinalizeRecording()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestStartRecordingWithAtmosCastPath(t *testing.T) {
	activeCast = nil
	castPath := filepath.Join(t.TempDir(), "env.cast")
	t.Setenv(EnvName, castPath)
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	err := StartRecordingIfRequested(
		newRecordingTestCommand("workflow"),
		&schema.AtmosConfiguration{},
		[]string{"workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected ATMOS_CAST path to start cast recording")
	}
	if activeCast.recorder.Path() != castPath {
		t.Fatalf("cast path = %q, want %q", activeCast.recorder.Path(), castPath)
	}
	FinalizeRecording()
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestAtmosCastRenderPathUsesOutputPlanner(t *testing.T) {
	output := filepath.Join(t.TempDir(), "env.gif")
	t.Setenv(EnvName, output)

	request, err := resolveRecordingRequest(newRecordingTestCommand("workflow"), &schema.AtmosConfiguration{}, []string{"workflow", "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if request.source != recordingSourceEnv || request.value != output || !request.hasPath() {
		t.Fatalf("unexpected request: %#v", request)
	}
	plan, err := planRecordingOutput(request.value, request.hasPath())
	if err != nil {
		t.Fatal(err)
	}
	if plan.castPath != "" || plan.castBasePath != os.TempDir() || plan.renderOutput != output || !plan.removeCast || plan.explicitCast {
		t.Fatalf("unexpected gif plan: %#v", plan)
	}
}

func TestAtmosCastFalseDoesNotRequestRecording(t *testing.T) {
	activeCast = nil
	t.Setenv(EnvName, "false")
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	err := StartRecordingIfRequested(
		newRecordingTestCommand("workflow"),
		&schema.AtmosConfiguration{},
		[]string{"workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("ATMOS_CAST=false should not request recording by itself")
	}
}

func TestAtmosCastFalseDoesNotOverrideConfigEnabledRecording(t *testing.T) {
	activeCast = nil
	basePath := t.TempDir()
	t.Setenv(EnvName, "false")
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	err := StartRecordingIfRequested(
		newRecordingTestCommand("workflow"),
		&schema.AtmosConfiguration{
			Cast: &schema.CastConfig{Recording: &schema.CastRecordingConfig{Enabled: true, BasePath: basePath}},
		},
		[]string{"workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("ATMOS_CAST=false should not disable config-enabled recording")
	}
	if !strings.HasPrefix(activeCast.recorder.Path(), basePath) {
		t.Fatalf("cast path = %q, want under %q", activeCast.recorder.Path(), basePath)
	}
	FinalizeRecording()
}

func TestCastFlagOverridesAtmosCast(t *testing.T) {
	activeCast = nil
	envPath := filepath.Join(t.TempDir(), "env.cast")
	flagPath := filepath.Join(t.TempDir(), "flag.cast")
	t.Setenv(EnvName, envPath)
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("workflow")
	if err := cmd.Flags().Set(FlagName, flagPath); err != nil {
		t.Fatal(err)
	}
	err := StartRecordingIfRequested(
		cmd,
		&schema.AtmosConfiguration{},
		[]string{"--cast=" + flagPath, "workflow", "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil || activeCast.recorder == nil {
		t.Fatal("expected explicit --cast to start recording")
	}
	if activeCast.recorder.Path() != flagPath {
		t.Fatalf("cast path = %q, want %q", activeCast.recorder.Path(), flagPath)
	}
	FinalizeRecording()
	if _, err := os.Stat(flagPath); err != nil {
		t.Fatalf("flag cast file missing: %v", err)
	}
	if _, err := os.Stat(envPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("env cast path should not be used, stat err: %v", err)
	}
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

func TestStartRecordingRecordsHelpWhenCastFlagExplicit(t *testing.T) {
	activeCast = nil
	castPath := filepath.Join(t.TempDir(), "help.cast")
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("about")
	cmd.Flags().Bool("help", false, "")
	if err := cmd.Flags().Set("help", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set(FlagName, castPath); err != nil {
		t.Fatal(err)
	}

	err := StartRecordingIfRequested(cmd, &schema.AtmosConfiguration{}, []string{"about", "--help", "--cast=" + castPath})
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil {
		t.Fatal("explicit --cast should record help output")
	}
	FinalizeRecording()
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestStartRecordingRecordsHelpWhenAtmosCastSet(t *testing.T) {
	activeCast = nil
	castPath := filepath.Join(t.TempDir(), "env-help.cast")
	t.Setenv(EnvName, castPath)
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("about")
	cmd.Flags().Bool("help", false, "")
	if err := cmd.Flags().Set("help", "true"); err != nil {
		t.Fatal(err)
	}

	err := StartRecordingIfRequested(cmd, &schema.AtmosConfiguration{}, []string{"about", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if activeCast == nil {
		t.Fatal("ATMOS_CAST should record help output")
	}
	FinalizeRecording()
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestStartRecordingStillSkipsHelpWithImplicitRecording(t *testing.T) {
	activeCast = nil
	t.Cleanup(func() {
		if activeCast != nil {
			FinalizeRecording()
		}
	})

	cmd := newRecordingTestCommand("about")
	cmd.Flags().Bool("help", false, "")
	if err := cmd.Flags().Set("help", "true"); err != nil {
		t.Fatal(err)
	}

	err := StartRecordingIfRequested(
		cmd,
		&schema.AtmosConfiguration{
			Cast: &schema.CastConfig{Recording: &schema.CastRecordingConfig{Enabled: true, BasePath: t.TempDir()}},
		},
		[]string{"about", "--help"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if activeCast != nil {
		t.Fatal("config-enabled recording must not capture help output")
	}
}

func TestPlanRecordingOutputAcceptsStaticRenderExtensions(t *testing.T) {
	for _, ext := range []string{".html", ".ascii", ".png", ".jpg", ".jpeg"} {
		output := filepath.Join(t.TempDir(), "demo"+ext)
		plan, err := planRecordingOutput(output, true)
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if plan.renderOutput != output || !plan.removeCast {
			t.Fatalf("%s: unexpected plan: %#v", ext, plan)
		}
	}
}

func TestRenderRecordedCastDispatchesStaticFormats(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	castContent := `{"version":3,"term":{"cols":10,"rows":2}}` + "\n" + `[0.1,"o","hello\n"]` + "\n"
	if err := os.WriteFile(castPath, []byte(castContent), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".html", ".ascii", ".png", ".jpg", ".jpeg"} {
		output := filepath.Join(t.TempDir(), "out"+ext)
		if err := renderRecordedCast(castPath, output); err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		info, err := os.Stat(output)
		if err != nil || info.Size() == 0 {
			t.Fatalf("%s: output missing or empty: %v", ext, err)
		}
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
	parser := pkgFlags.NewStandardParser(
		pkgFlags.WithStringFlag(FlagName, "", "", ""),
		pkgFlags.WithNoOptDefValNoSpaceValue(FlagName, autoFlagValue),
	)
	parser.RegisterFlags(cmd)
	return cmd
}
