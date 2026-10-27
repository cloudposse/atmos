package asciicast

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writeMarkerTestCast writes a minimal asciicast v3 file where each event
// carries its own explicit relative timestamp, so marker-cutoff ordering can
// be tested precisely (writeTestCast in cellgrid_test.go hardcodes every
// event to the same timestamp, which can't exercise cutoff behavior).
func writeMarkerTestCast(t *testing.T, width, height int, events [][3]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.cast")
	content := fmt.Sprintf(`{"version":3,"term":{"cols":%d,"rows":%d}}`+"\n", width, height)
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		content += string(encoded) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderMarkerScreenshotsIsNoopWithoutMarkers(t *testing.T) {
	path := writeTestCast(t, 20, 5, "hello\n")
	if err := RenderMarkerScreenshots(path); err != nil {
		t.Fatalf("RenderMarkerScreenshots error: %v", err)
	}
}

func TestRenderMarkerScreenshotsWritesOnePNGPerMarker(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.png")
	second := filepath.Join(dir, "second.png")
	path := writeMarkerTestCast(t, 20, 5, [][3]any{
		{0.1, "o", "one\n"},
		{0.2, "m", first},
		{0.3, "o", "two\n"},
		{0.4, "m", second},
	})

	if err := RenderMarkerScreenshots(path); err != nil {
		t.Fatalf("RenderMarkerScreenshots error: %v", err)
	}

	for _, out := range []string{first, second} {
		file, err := os.Open(out)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", out, err)
		}
		if _, err := png.Decode(file); err != nil {
			t.Fatalf("expected %s to be a valid PNG: %v", out, err)
		}
		_ = file.Close()
	}
}

func TestRenderMarkerScreenshotsCreatesMissingParentDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "deep", "hero.png")
	path := writeMarkerTestCast(t, 20, 5, [][3]any{
		{0.1, "o", "one\n"},
		{0.2, "m", target},
	})

	if err := RenderMarkerScreenshots(path); err != nil {
		t.Fatalf("RenderMarkerScreenshots error: %v", err)
	}

	file, err := os.Open(target)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", target, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := png.Decode(file); err != nil {
		t.Fatalf("expected %s to be a valid PNG: %v", target, err)
	}
}

func TestRenderMarkerScreenshotsReturnsErrorOnMissingInput(t *testing.T) {
	err := RenderMarkerScreenshots(filepath.Join(t.TempDir(), "missing.cast"))
	if err == nil {
		t.Fatal("expected an error for a missing input file")
	}
}

func TestRenderMarkerScreenshotsReturnsErrScreenshotRenderFailed(t *testing.T) {
	tests := []struct {
		name string
		// target builds the marker's output path inside dir.
		target func(t *testing.T, dir string) string
	}{
		{
			name: "parent directory cannot be created because a file is in the way",
			target: func(t *testing.T, dir string) string {
				blocker := filepath.Join(dir, "blocker")
				if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(blocker, "shot.png")
			},
		},
		{
			name: "output path is an existing directory so the rename fails",
			target: func(t *testing.T, dir string) string {
				existing := filepath.Join(dir, "shot.png")
				if err := os.Mkdir(existing, 0o755); err != nil {
					t.Fatal(err)
				}
				return existing
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			target := tt.target(t, dir)
			path := writeMarkerTestCast(t, 20, 5, [][3]any{
				{0.1, "o", "one\n"},
				{0.2, "m", target},
			})

			err := RenderMarkerScreenshots(path)
			if !errors.Is(err, ErrScreenshotRenderFailed) {
				t.Fatalf("expected ErrScreenshotRenderFailed, got %v", err)
			}

			// A failed render must not leave temp files behind next to the target.
			leftovers, globErr := filepath.Glob(filepath.Join(dir, ".screenshot-*.png.tmp"))
			if globErr != nil {
				t.Fatal(globErr)
			}
			if len(leftovers) != 0 {
				t.Fatalf("expected no leftover temp files, found %v", leftovers)
			}
		})
	}
}
