package asciicast

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRenderReportsToolchainResolutionFailure(t *testing.T) {
	want := errors.New("toolchain unavailable")
	withRenderToolResolver(t, func(renderToolRequirements) (renderTools, error) {
		return renderTools{}, want
	})
	err := Render("input.cast", &RenderOptions{GIF: filepath.Join(t.TempDir(), "out.gif")})
	if !errors.Is(err, want) {
		t.Fatalf("expected toolchain error, got %v", err)
	}
}

func TestRenderNoTargetsSucceeds(t *testing.T) {
	if err := Render("input.cast", &RenderOptions{}); err != nil {
		t.Fatalf("render without targets: %v", err)
	}
}

func TestRenderStaticTargetSkipsToolResolution(t *testing.T) {
	called := false
	withRenderToolResolver(t, func(renderToolRequirements) (renderTools, error) {
		called = true
		return renderTools{}, errors.New("static outputs must not resolve renderers")
	})

	input := writeTestCast(t, 20, 3, "hello\\n")
	output := filepath.Join(t.TempDir(), "out.ascii")
	if err := Render(input, &RenderOptions{ASCII: output}); err != nil {
		t.Fatalf("render static ASCII: %v", err)
	}
	if called {
		t.Fatal("static output should not resolve renderer tools")
	}
}

func TestRenderRejectsExistingOutputBeforeRendering(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out.gif")
	if err := os.WriteFile(output, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	withRenderToolResolver(t, func(renderToolRequirements) (renderTools, error) {
		called = true
		return renderTools{}, nil
	})
	err := Render("input.cast", &RenderOptions{GIF: output})
	if !errors.Is(err, ErrRenderOutputExists) {
		t.Fatalf("expected output exists error, got %v", err)
	}
	if called {
		t.Fatal("renderer tools should not resolve when output exists")
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
	if targets[0].format != renderFormatGIF || targets[1].format != renderFormatMP4 {
		t.Fatalf("unexpected render target formats: %#v", targets)
	}
}

func TestRenderWithFakeAgg(t *testing.T) {
	bin := t.TempDir()
	installFakeTool(t, bin, "agg")
	t.Setenv(asciicastHelperEnv, "1")
	withRenderToolResolver(t, func(requirements renderToolRequirements) (renderTools, error) {
		if !requirements.agg || requirements.ffmpeg {
			t.Fatalf("requirements = %#v", requirements)
		}
		return renderTools{agg: filepath.Join(bin, helperExecutableName("agg"))}, nil
	})

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

func TestRenderMP4ReportsToolchainFailure(t *testing.T) {
	want := errors.New("toolchain unavailable")
	withRenderToolResolver(t, func(requirements renderToolRequirements) (renderTools, error) {
		if !requirements.agg || !requirements.ffmpeg {
			t.Fatalf("requirements = %#v", requirements)
		}
		return renderTools{}, want
	})
	err := Render("input.cast", &RenderOptions{MP4: filepath.Join(t.TempDir(), "out.mp4")})
	if !errors.Is(err, want) {
		t.Fatalf("expected toolchain error, got %v", err)
	}
}

func TestRenderMP4WithFakeTools(t *testing.T) {
	bin := t.TempDir()
	installFakeTool(t, bin, "agg")
	installFakeTool(t, bin, "ffmpeg")
	t.Setenv(asciicastHelperEnv, "1")
	withRenderToolResolver(t, func(requirements renderToolRequirements) (renderTools, error) {
		if !requirements.agg || !requirements.ffmpeg {
			t.Fatalf("requirements = %#v", requirements)
		}
		return renderTools{
			agg:    filepath.Join(bin, helperExecutableName("agg")),
			ffmpeg: filepath.Join(bin, helperExecutableName("ffmpeg")),
		}, nil
	})

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

func TestRenderResolvesAnimatedToolsOnce(t *testing.T) {
	bin := t.TempDir()
	installFakeTool(t, bin, "agg")
	installFakeTool(t, bin, "ffmpeg")
	t.Setenv(asciicastHelperEnv, "1")

	calls := 0
	withRenderToolResolver(t, func(requirements renderToolRequirements) (renderTools, error) {
		calls++
		if !requirements.agg || !requirements.ffmpeg {
			t.Fatalf("requirements = %#v", requirements)
		}
		return renderTools{
			agg:    filepath.Join(bin, helperExecutableName("agg")),
			ffmpeg: filepath.Join(bin, helperExecutableName("ffmpeg")),
		}, nil
	})

	if err := Render("input.cast", &RenderOptions{
		GIF: filepath.Join(t.TempDir(), "out.gif"),
		MP4: filepath.Join(t.TempDir(), "out.mp4"),
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("renderer resolver calls = %d, want 1", calls)
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

func TestRenderMP4PropagatesAggFailure(t *testing.T) {
	// A missing managed agg binary must stop the MP4 pipeline before ffmpeg runs.
	bin := t.TempDir()
	installFakeTool(t, bin, "ffmpeg")
	t.Setenv(asciicastHelperEnv, "1")
	withRenderToolResolver(t, func(renderToolRequirements) (renderTools, error) {
		return renderTools{
			agg:    filepath.Join(bin, "missing-agg"),
			ffmpeg: filepath.Join(bin, helperExecutableName("ffmpeg")),
		}, nil
	})

	output := filepath.Join(t.TempDir(), "out.mp4")
	err := Render("input.cast", &RenderOptions{MP4: output})
	if err == nil {
		t.Fatal("expected agg execution error")
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ffmpeg should not create output after agg failure: %v", statErr)
	}
}

func TestRenderPropagatesAggFailure(t *testing.T) {
	withRenderToolResolver(t, func(renderToolRequirements) (renderTools, error) {
		return renderTools{agg: filepath.Join(t.TempDir(), "missing-agg")}, nil
	})
	output := filepath.Join(t.TempDir(), "out.gif")
	err := Render("input.cast", &RenderOptions{GIF: output})
	if err == nil {
		t.Fatal("expected agg execution error")
	}
}

func TestRenderToolSpecs(t *testing.T) {
	tests := []struct {
		name         string
		requirements renderToolRequirements
		want         []renderToolSpec
	}{
		{
			name: "gif",
			requirements: renderToolRequirements{
				agg: true,
			},
			want: []renderToolSpec{{dependency: aggTool, version: aggVersion, binary: "agg"}},
		},
		{
			name: "mp4",
			requirements: renderToolRequirements{
				agg:    true,
				ffmpeg: true,
			},
			want: []renderToolSpec{
				{dependency: aggTool, version: aggVersion, binary: "agg"},
				{dependency: ffmpegTool, version: ffmpegVersion, binary: "ffmpeg"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderToolSpecs(tt.requirements); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("renderToolSpecs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveRenderToolsReportsInstallationFailure(t *testing.T) {
	want := errors.New("install failed")
	withRenderToolchainHooks(
		t,
		func(map[string]string) error { return want },
		func(renderToolSpec) (string, error) {
			t.Fatal("should not locate tools after install failure")
			return "", nil
		},
	)

	_, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true})
	if !errors.Is(err, want) || !errors.Is(err, errUtils.ErrToolInstall) {
		t.Fatalf("expected wrapped install failure, got %v", err)
	}
}

func TestResolveRenderToolsRequestsRequiredDependencies(t *testing.T) {
	tests := []struct {
		name         string
		requirements renderToolRequirements
		want         map[string]string
	}{
		{
			name:         "gif requests agg only",
			requirements: renderToolRequirements{agg: true},
			want:         map[string]string{aggTool: aggVersion},
		},
		{
			name:         "mp4 requests agg and ffmpeg",
			requirements: renderToolRequirements{agg: true, ffmpeg: true},
			want:         map[string]string{aggTool: aggVersion, ffmpegTool: ffmpegVersion},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]string
			withRenderToolchainHooks(
				t,
				func(deps map[string]string) error {
					got = make(map[string]string, len(deps))
					for dependency, version := range deps {
						got[dependency] = version
					}
					return nil
				},
				func(spec renderToolSpec) (string, error) { return filepath.Join(t.TempDir(), spec.binary), nil },
			)

			if _, err := resolveRenderToolsFromToolchain(tt.requirements); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("requested dependencies = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveRenderToolsReportsResolutionFailure(t *testing.T) {
	want := errors.New("binary not found")
	withRenderToolchainHooks(
		t,
		func(map[string]string) error { return nil },
		func(renderToolSpec) (string, error) { return "", want },
	)

	_, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true})
	if !errors.Is(err, want) || !errors.Is(err, errUtils.ErrToolInstall) {
		t.Fatalf("expected wrapped resolution failure, got %v", err)
	}
}

func TestResolveRenderToolsUsesAbsoluteBinaryPaths(t *testing.T) {
	withRenderToolchainHooks(
		t,
		func(map[string]string) error { return nil },
		func(spec renderToolSpec) (string, error) { return filepath.Join("relative", spec.binary), nil },
	)

	tools, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true, ffmpeg: true})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(tools.agg) || !filepath.IsAbs(tools.ffmpeg) {
		t.Fatalf("renderer paths must be absolute: %#v", tools)
	}
}

// TestResolveRenderToolsSkipsInstallWhenNoToolsRequired asserts the
// short-circuit at the top of resolveRenderToolsFromToolchain: a static
// render target (ASCII/HTML/PNG/JPEG) produces a zero-value
// renderToolRequirements, and that must return immediately without ever
// invoking the dependency installer or binary resolver hooks -- reaching
// either would mean an unnecessary (and possibly network-dependent) tool
// install was triggered for a render that needs no managed renderer at all.
func TestResolveRenderToolsSkipsInstallWhenNoToolsRequired(t *testing.T) {
	withRenderToolchainHooks(
		t,
		func(map[string]string) error {
			t.Fatal("should not install dependencies when no renderer tools are required")
			return nil
		},
		func(renderToolSpec) (string, error) {
			t.Fatal("should not resolve binaries when no renderer tools are required")
			return "", nil
		},
	)

	tools, err := resolveRenderToolsFromToolchain(renderToolRequirements{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tools != (renderTools{}) {
		t.Fatalf("expected zero-value renderTools, got %#v", tools)
	}
}

func withRenderToolResolver(t *testing.T, resolver func(renderToolRequirements) (renderTools, error)) {
	t.Helper()
	previous := resolveRenderTools
	resolveRenderTools = resolver
	t.Cleanup(func() { resolveRenderTools = previous })
}

func withRenderToolchainHooks(t *testing.T, ensure func(map[string]string) error, find func(renderToolSpec) (string, error)) {
	t.Helper()
	previousEnsure := ensureRenderToolDependencies
	previousFind := findRenderToolBinary
	ensureRenderToolDependencies = ensure
	findRenderToolBinary = find
	t.Cleanup(func() {
		ensureRenderToolDependencies = previousEnsure
		findRenderToolBinary = previousFind
	})
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
