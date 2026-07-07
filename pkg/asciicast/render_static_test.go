package asciicast

import (
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

func TestRenderASCIIWritesPlainText(t *testing.T) {
	cast := writeTestCast(t, 20, 5, "\x1b[1;32mSTACKS\x1b[0m\n", "  dev\n  prod\n")
	out := filepath.Join(t.TempDir(), "out.ascii")
	if err := RenderASCII(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "STACKS\n  dev\n  prod\n"
	if string(got) != want {
		t.Fatalf("ascii = %q, want %q", got, want)
	}
}

func TestRenderHTMLEmitsStyledSpans(t *testing.T) {
	cast := writeTestCast(t, 20, 3, "\x1b[1;32mok\x1b[0m plain <tag>\n")
	out := filepath.Join(t.TempDir(), "out.html")
	if err := RenderHTML(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	html := string(got)
	for _, want := range []string{
		"font-weight:bold",
		">ok</span>",
		" plain &lt;tag&gt;",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "background-color") {
		t.Fatalf("html must not contain background colors:\n%s", html)
	}
}

func TestRenderHTMLMapsANSIBlueToAtmosBlue(t *testing.T) {
	cast := writeTestCast(t, 20, 3, "\x1b[34mblue\x1b[0m and \x1b[94mbright\x1b[0m\n")
	out := filepath.Join(t.TempDir(), "out.html")
	if err := RenderHTML(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(got), "color:"+atmosBlue); count != 2 {
		t.Fatalf("atmos blue occurrences = %d, want 2:\n%s", count, got)
	}
}

func TestImageColorMapsANSIBlueToAtmosBlue(t *testing.T) {
	if got := imageColor(ansi.Blue); got != imageAtmosBlue {
		t.Fatalf("blue = %#v, want Atmos blue %#v", got, imageAtmosBlue)
	}
	if got := imageColor(ansi.BrightBlue); got != imageAtmosBlue {
		t.Fatalf("bright blue = %#v, want Atmos blue %#v", got, imageAtmosBlue)
	}

	red := imageColor(ansi.Red)
	if red == imageAtmosBlue {
		t.Fatal("red should not map to Atmos blue")
	}
}

func TestDimColorPreservesAlpha(t *testing.T) {
	got := dimColor(color.RGBA{R: 100, G: 50, B: 25, A: 200})
	want := color.RGBA{R: 60, G: 30, B: 15, A: 200}
	if got != want {
		t.Fatalf("dim color = %#v, want %#v", got, want)
	}
}

func TestRenderHTMLEmitsHyperlinks(t *testing.T) {
	cast := writeTestCast(t, 30, 3, "\x1b]8;;https://atmos.tools\x07docs\x1b]8;;\x07\n")
	out := filepath.Join(t.TempDir(), "out.html")
	if err := RenderHTML(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `<a href="https://atmos.tools">docs</a>`) {
		t.Fatalf("hyperlink missing:\n%s", got)
	}
}

func TestRenderPNGProducesDecodableImage(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "hi\n")
	out := filepath.Join(t.TempDir(), "out.png")
	if err := RenderPNG(cast, out); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 2*imagePadding || bounds.Dy() <= 2*imagePadding {
		t.Fatalf("image too small: %v", bounds)
	}
	// The canvas must be filled with the terminal background, not transparent.
	r, g, b, a := img.At(1, 1).RGBA()
	wr, wg, wb, wa := imageBackground.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Fatalf("corner pixel = (%d,%d,%d,%d), want background", r, g, b, a)
	}
}

func TestRenderJPEGProducesDecodableImage(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "hi\n")
	out := filepath.Join(t.TempDir(), "out.jpg")
	if err := RenderJPEG(cast, out); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	img, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() <= 2*imagePadding {
		t.Fatalf("image too small: %v", img.Bounds())
	}
}

func TestRenderHTMLStyleCombinations(t *testing.T) {
	tests := []struct {
		name string
		sgr  string
		want []string
		none []string
	}{
		{
			name: "italic",
			sgr:  "\x1b[3m",
			want: []string{"font-style:italic"},
		},
		{
			name: "faint",
			sgr:  "\x1b[2m",
			want: []string{"opacity:0.7"},
		},
		{
			name: "underline",
			sgr:  "\x1b[4m",
			want: []string{"text-decoration:underline"},
		},
		{
			name: "strikethrough",
			sgr:  "\x1b[9m",
			want: []string{"text-decoration:line-through"},
		},
		{
			name: "underline and strikethrough combine decorations",
			sgr:  "\x1b[4;9m",
			want: []string{"text-decoration:underline line-through"},
		},
		{
			name: "no style produces no span",
			sgr:  "",
			want: []string{"plain"},
			none: []string{"<span"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cast := writeTestCast(t, 20, 3, tt.sgr+"plain\x1b[0m\n")
			out := filepath.Join(t.TempDir(), "out.html")
			if err := RenderHTML(cast, out); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			html := string(got)
			for _, want := range tt.want {
				if !strings.Contains(html, want) {
					t.Fatalf("html missing %q:\n%s", want, html)
				}
			}
			for _, notWant := range tt.none {
				if strings.Contains(html, notWant) {
					t.Fatalf("html unexpectedly contains %q:\n%s", notWant, html)
				}
			}
		})
	}
}

func TestRenderHTMLColorOverrideAppliesToResolvedHex(t *testing.T) {
	// bat's GitHub blue (#183691) must be remapped to the Atmos brand blue via
	// htmlColorOverrides, exercising the override-hit branch of cssColor
	// (distinct from the ansi.BasicColor blue fast path already covered by
	// TestRenderHTMLMapsANSIBlueToAtmosBlue).
	hex, ok := htmlColorOverrides["#183691"]
	if !ok {
		t.Fatal("expected #183691 override to be registered")
	}
	if hex != atmosBlue {
		t.Fatalf("override = %q, want %q", hex, atmosBlue)
	}

	got := cssColor(ansi.RGBColor{R: 0x18, G: 0x36, B: 0x91})
	if got != atmosBlue {
		t.Fatalf("cssColor override = %q, want %q", got, atmosBlue)
	}
}

func TestCssColorNilReturnsEmptyString(t *testing.T) {
	if got := cssColor(nil); got != "" {
		t.Fatalf("cssColor(nil) = %q, want empty string", got)
	}
}

func TestColorHexFormatsRGBAsLowercaseHex(t *testing.T) {
	// colorHex itself has no nil guard (nil-safety is cssColor's
	// responsibility, covered by TestCssColorNilReturnsEmptyString); it only
	// needs to format a resolved color.Color as "#rrggbb".
	got := colorHex(ansi.RGBColor{R: 0x18, G: 0x36, B: 0x91})
	if got != "#183691" {
		t.Fatalf("colorHex = %q, want %q", got, "#183691")
	}
}

func TestTextDecorationsEmptyWhenNoDecorations(t *testing.T) {
	style := cellbuf.Style{}
	decorations := textDecorations(&style)
	if len(decorations) != 0 {
		t.Fatalf("decorations = %v, want none", decorations)
	}
}

func TestRenderHTMLSkipsNilAndZeroWidthCells(t *testing.T) {
	// A wide rune (e.g. CJK) followed by its zero-width continuation cell
	// exercises the `cell == nil || cell.Width == 0` skip in the draw loop.
	cast := writeTestCast(t, 10, 2, "你好\n") // "ni hao" - two wide runes
	out := filepath.Join(t.TempDir(), "out.html")
	if err := RenderHTML(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "你好") {
		t.Fatalf("wide runes missing from html: %q", got)
	}
}

func TestRenderHTMLReturnsErrorOnMissingInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.html")
	err := RenderHTML(filepath.Join(t.TempDir(), "does-not-exist.cast"), out)
	if err == nil {
		t.Fatal("expected error for missing input cast file")
	}
}

func TestRenderASCIIReturnsErrorOnMissingInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.ascii")
	err := RenderASCII(filepath.Join(t.TempDir(), "does-not-exist.cast"), out)
	if err == nil {
		t.Fatal("expected error for missing input cast file")
	}
}

func TestRenderASCIISkipsNilAndZeroWidthCells(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "你好\n")
	out := filepath.Join(t.TempDir(), "out.ascii")
	if err := RenderASCII(cast, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "你好\n" {
		t.Fatalf("ascii = %q, want wide runes with no duplication from zero-width continuations", got)
	}
}

func TestRenderDispatchesAllStaticTargets(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "hello\n")
	dir := t.TempDir()
	opts := RenderOptions{
		HTML:  filepath.Join(dir, "out.html"),
		ASCII: filepath.Join(dir, "out.ascii"),
		PNG:   filepath.Join(dir, "out.png"),
		JPEG:  filepath.Join(dir, "out.jpg"),
	}
	if err := Render(cast, &opts); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{opts.HTML, opts.ASCII, opts.PNG, opts.JPEG} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("missing output %s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("empty output %s", path)
		}
	}
}
