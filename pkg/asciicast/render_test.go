package asciicast

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderReportsMissingAgg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Render("input.cast", &RenderOptions{GIF: filepath.Join(t.TempDir(), "out.gif")})
	if err == nil {
		t.Fatal("expected missing agg error")
	}
	if !strings.Contains(err.Error(), "missing required tool `agg`") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderNoTargetsSucceeds(t *testing.T) {
	if err := Render("input.cast", &RenderOptions{}); err != nil {
		t.Fatalf("render without targets: %v", err)
	}
}

func TestRenderRejectsExistingOutputBeforeRendering(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out.gif")
	if err := os.WriteFile(output, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Render("input.cast", &RenderOptions{GIF: output})
	if !errors.Is(err, ErrRenderOutputExists) {
		t.Fatalf("expected output exists error, got %v", err)
	}
}

func TestPrepareRenderOutputCreatesParentDirectories(t *testing.T) {
	output := filepath.Join(t.TempDir(), "nested", "out.gif")
	if err := prepareRenderOutput(output); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Dir(output)); err != nil || !info.IsDir() {
		t.Fatalf("parent directory was not created: %v", err)
	}
}

func TestRenderTargetsOrderAndTypes(t *testing.T) {
	targets := renderTargets(&RenderOptions{GIF: "out.gif", MP4: "out.mp4"})
	if len(targets) != 2 {
		t.Fatalf("target count = %d", len(targets))
	}
	for i, want := range []string{"out.gif", "out.mp4"} {
		if targets[i].output != want {
			t.Fatalf("target[%d] = %q, want %q", i, targets[i].output, want)
		}
	}
}

func TestRenderWithFakeAgg(t *testing.T) {
	bin := t.TempDir()
	installFakeTool(t, bin, "agg")
	t.Setenv("PATH", bin)
	t.Setenv(asciicastHelperEnv, "1")

	output := filepath.Join(t.TempDir(), "out.gif")
	if err := Render("input.cast", &RenderOptions{GIF: output}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "agg:input.cast:") {
		t.Fatalf("fake agg output = %q", content)
	}
}

func TestRenderMP4ReportsMissingFFmpegBeforeAgg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Render("input.cast", &RenderOptions{MP4: filepath.Join(t.TempDir(), "out.mp4")})
	if !errors.Is(err, ErrMissingFFmpeg) {
		t.Fatalf("expected missing ffmpeg, got %v", err)
	}
}

func TestRenderMP4WithFakeTools(t *testing.T) {
	bin := t.TempDir()
	installFakeTool(t, bin, "agg")
	installFakeTool(t, bin, "ffmpeg")
	t.Setenv("PATH", bin)
	t.Setenv(asciicastHelperEnv, "1")

	output := filepath.Join(t.TempDir(), "out.mp4")
	if err := Render("input.cast", &RenderOptions{MP4: output}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "mp4" {
		t.Fatalf("mp4 output = %q", content)
	}
}

func TestReadEventsRejectsOversizedEventLine(t *testing.T) {
	// A single event token larger than maxEventTokenSize forces the scanner
	// to give up mid-scan with bufio.ErrTooLong, exercising the
	// scanner.Err() failure path (distinct from a clean EOF).
	path := filepath.Join(t.TempDir(), "oversized.cast")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"version":3,"term":{"cols":80,"rows":24}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", maxEventTokenSize+1)
	encoded, err := json.Marshal([]any{0.1, "o", huge})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, _, err = ReadEvents(path)
	if err == nil {
		t.Fatal("expected scanner error for oversized event line")
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected bufio.ErrTooLong, got %v", err)
	}
}

// errWriter is a pure-Go io.Writer stand-in for an OS-level write failure
// (e.g. a closed pipe or full disk), used to exercise io.WriteString error
// propagation without any platform-specific binary or file-descriptor trick.
type errWriter struct {
	failAfter int
	writes    int
}

var errSimulatedWrite = errors.New("simulated write failure")

func (w *errWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.failAfter {
		return 0, errSimulatedWrite
	}
	return len(p), nil
}

func TestPlayPropagatesReadEventsError(t *testing.T) {
	err := Play(filepath.Join(t.TempDir(), "does-not-exist.cast"), &strings.Builder{})
	if err == nil {
		t.Fatal("expected error for missing cast file")
	}
}

func TestPlayPropagatesWriteError(t *testing.T) {
	cast := writeTestCast(t, 20, 3, "first\n", "second\n")
	w := &errWriter{failAfter: 0}
	err := Play(cast, w)
	if !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected simulated write error, got %v", err)
	}
}

func TestPlaySucceedsWithWorkingWriter(t *testing.T) {
	cast := writeTestCast(t, 20, 3, "first\n")
	var sb strings.Builder
	if err := Play(cast, &sb); err != nil {
		t.Fatal(err)
	}
	if sb.String() != "first\n" {
		t.Fatalf("played output = %q", sb.String())
	}
}

func TestRenderNilOptionsReturnsNil(t *testing.T) {
	if err := Render("input.cast", nil); err != nil {
		t.Fatalf("Render with nil opts: %v", err)
	}
}

func TestPrepareRenderOutputBareFilenameSkipsMkdirAll(t *testing.T) {
	// filepath.Dir("out.gif") == "." — this must short-circuit without
	// attempting to create a directory.
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	if err := prepareRenderOutput("out.gif"); err != nil {
		t.Fatalf("prepareRenderOutput bare filename: %v", err)
	}
}

func TestRenderMP4PropagatesAggFailureAfterFFmpegFound(t *testing.T) {
	// ffmpeg is present but agg is not: renderMP4 must surface renderWithAgg's
	// error (the temp-gif render step) rather than proceeding to invoke ffmpeg.
	bin := t.TempDir()
	installFakeTool(t, bin, "ffmpeg")
	t.Setenv("PATH", bin)
	t.Setenv(asciicastHelperEnv, "1")

	output := filepath.Join(t.TempDir(), "out.mp4")
	err := Render("input.cast", &RenderOptions{MP4: output})
	if !errors.Is(err, ErrMissingAgg) {
		t.Fatalf("expected missing agg error, got %v", err)
	}
}

func TestRenderPropagatesAggFailure(t *testing.T) {
	// renderWithAgg's error must propagate through Render's target loop
	// (renderTargets -> target.render -> Render's error return).
	t.Setenv("PATH", t.TempDir())
	output := filepath.Join(t.TempDir(), "out.gif")
	err := Render("input.cast", &RenderOptions{GIF: output})
	if !errors.Is(err, ErrMissingAgg) {
		t.Fatalf("expected missing agg error propagated from Render, got %v", err)
	}
}

var _ io.Writer = (*errWriter)(nil)

func installFakeTool(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, helperExecutableName(name))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}
