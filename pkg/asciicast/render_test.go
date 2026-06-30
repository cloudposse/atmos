package asciicast

import (
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
