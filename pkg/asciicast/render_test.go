package asciicast

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderReportsMissingAgg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Render("input.cast", RenderOptions{GIF: filepath.Join(t.TempDir(), "out.gif")})
	if err == nil {
		t.Fatal("expected missing agg error")
	}
	if !strings.Contains(err.Error(), "missing required tool `agg`") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderNoTargetsSucceeds(t *testing.T) {
	if err := Render("input.cast", RenderOptions{}); err != nil {
		t.Fatalf("render without targets: %v", err)
	}
}

func TestRenderRejectsExistingOutputBeforeRendering(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out.svg")
	if err := os.WriteFile(output, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Render("input.cast", RenderOptions{SVG: output})
	if !errors.Is(err, ErrRenderOutputExists) {
		t.Fatalf("expected output exists error, got %v", err)
	}
}

func TestPrepareRenderOutputCreatesParentDirectories(t *testing.T) {
	output := filepath.Join(t.TempDir(), "nested", "out.svg")
	if err := prepareRenderOutput(output); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Dir(output)); err != nil || !info.IsDir() {
		t.Fatalf("parent directory was not created: %v", err)
	}
}

func TestRenderTargetsOrderAndTypes(t *testing.T) {
	targets := renderTargets(RenderOptions{SVG: "out.svg", GIF: "out.gif", MP4: "out.mp4"})
	if len(targets) != 3 {
		t.Fatalf("target count = %d", len(targets))
	}
	for i, want := range []string{"out.svg", "out.gif", "out.mp4"} {
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
	if err := Render("input.cast", RenderOptions{GIF: output}); err != nil {
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

func TestRenderSVGReportsMissingRenderer(t *testing.T) {
	err := Render("input.cast", RenderOptions{SVG: filepath.Join(t.TempDir(), "out.svg")})
	if !errors.Is(err, ErrMissingSVGRenderer) {
		t.Fatalf("expected missing svg renderer, got %v", err)
	}
}

func TestRenderMP4ReportsMissingFFmpegBeforeAgg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Render("input.cast", RenderOptions{MP4: filepath.Join(t.TempDir(), "out.mp4")})
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
	if err := Render("input.cast", RenderOptions{MP4: output}); err != nil {
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
