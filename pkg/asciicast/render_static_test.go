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
