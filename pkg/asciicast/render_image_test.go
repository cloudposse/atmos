package asciicast

import (
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

// styledBuffer builds a single-row cell grid where each rune in text gets the
// corresponding style from styles (by index). Missing styles default to the
// zero value. This mirrors how render_test.go/render_static_test.go build
// minimal fixtures via writeTestCast, but render_image tests need direct
// control over cellbuf.Style fields (Bg/UlStyle/Attrs) that cannot be
// expressed through raw SGR sequences in every combination under test.
func styledBuffer(t *testing.T, text string, styles map[int]cellbuf.Style) *cellbuf.Buffer {
	t.Helper()
	runes := []rune(text)
	buf := cellbuf.NewBuffer(len(runes), 1)
	for i, r := range runes {
		cell := cellbuf.NewCell(r)
		if s, ok := styles[i]; ok {
			cell.Style = s
		}
		buf.SetCell(i, 0, cell)
	}
	return buf
}

func TestGlyphFacesForStyle(t *testing.T) {
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		style cellbuf.Style
		want  func(f *glyphFaces) interface{}
	}{
		{
			name:  "bold and italic",
			style: *(&cellbuf.Style{}).Bold(true).Italic(true),
			want:  func(f *glyphFaces) interface{} { return f.boldItalic },
		},
		{
			name:  "bold only",
			style: *(&cellbuf.Style{}).Bold(true),
			want:  func(f *glyphFaces) interface{} { return f.bold },
		},
		{
			name:  "italic only",
			style: *(&cellbuf.Style{}).Italic(true),
			want:  func(f *glyphFaces) interface{} { return f.italic },
		},
		{
			name:  "neither",
			style: cellbuf.Style{},
			want:  func(f *glyphFaces) interface{} { return f.regular },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := faces.forStyle(&tt.style)
			want := tt.want(faces)
			if got != want {
				t.Fatalf("forStyle(%+v) = %p, want %p", tt.style, got, want)
			}
		})
	}
}

func TestCellColorsBackgroundFill(t *testing.T) {
	style := cellbuf.Style{Bg: ansi.RGBColor{R: 0x00, G: 0x00, B: 0xff}}
	_, bg := cellColors(&style)
	if bg == nil {
		t.Fatal("expected non-nil background")
	}
	r, g, b, _ := bg.RGBA()
	if r != 0 || g != 0 || b == 0 {
		t.Fatalf("background = (%d,%d,%d), want blue", r, g, b)
	}
}

func TestCellColorsFgNilUsesDefault(t *testing.T) {
	fg, bg := cellColors(&cellbuf.Style{})
	if fg != imageForeground {
		t.Fatalf("fg = %#v, want default imageForeground", fg)
	}
	if bg != nil {
		t.Fatalf("bg = %#v, want nil", bg)
	}
}

func TestCellColorsFgSet(t *testing.T) {
	style := cellbuf.Style{Fg: ansi.RGBColor{R: 0x00, G: 0xff, B: 0x00}}
	fg, _ := cellColors(&style)
	r, g, b, _ := fg.RGBA()
	if g == 0 || r != 0 || b != 0 {
		t.Fatalf("fg = (%d,%d,%d), want green", r, g, b)
	}
}

func TestCellColorsReverseSwapsFgBg(t *testing.T) {
	style := cellbuf.Style{
		Fg: ansi.RGBColor{R: 0xff, G: 0x00, B: 0x00},
		Bg: ansi.RGBColor{R: 0x00, G: 0xff, B: 0x00},
	}
	style.Reverse(true)

	fg, bg := cellColors(&style)
	// fg should now carry the original background color (green), and bg
	// should carry the original foreground color (red).
	fr, fg2, fb, _ := fg.RGBA()
	if fg2 == 0 || fr != 0 || fb != 0 {
		t.Fatalf("reversed fg = (%d,%d,%d), want green (from original bg)", fr, fg2, fb)
	}
	br, bgc, bb, _ := bg.RGBA()
	if br == 0 || bgc != 0 || bb != 0 {
		t.Fatalf("reversed bg = (%d,%d,%d), want red (from original fg)", br, bgc, bb)
	}
}

func TestCellColorsReverseWithNilBackground(t *testing.T) {
	// Reverse video with no explicit background must not panic and must
	// fall back to the canvas background for the swapped bg slot's fg role.
	style := cellbuf.Style{Fg: ansi.RGBColor{R: 0xff, G: 0x00, B: 0x00}}
	style.Reverse(true)

	fg, bg := cellColors(&style)
	if fg != imageBackground {
		t.Fatalf("reversed fg with nil bg = %#v, want imageBackground", fg)
	}
	if bg == nil {
		t.Fatal("reversed bg should carry the original fg color, not nil")
	}
	r, g, b, _ := bg.RGBA()
	if r == 0 || g != 0 || b != 0 {
		t.Fatalf("reversed bg = (%d,%d,%d), want red (from original fg)", r, g, b)
	}
}

func TestCellColorsFaintDimsForeground(t *testing.T) {
	style := cellbuf.Style{Fg: ansi.RGBColor{R: 0xff, G: 0xff, B: 0xff}}
	style.Faint(true)

	fg, _ := cellColors(&style)
	r, g, b, _ := fg.RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	got8 := uint8(r >> eightBitShift & byteMask)
	if got8 == 0xff || got8 == 0 {
		t.Fatalf("faint fg red channel = %d, want dimmed (< 255, > 0)", got8)
	}
	_ = g
	_ = b
}

func TestRasterizeGridSkipsZeroWidthAndNilCells(t *testing.T) {
	buf := cellbuf.NewBuffer(3, 1)
	// Put a wide-ish placeholder then explicitly clear one cell to nil width.
	cell := cellbuf.NewCell('A')
	buf.SetCell(0, 0, cell)
	zero := cellbuf.NewCell('A')
	zero.Width = 0
	buf.SetCell(1, 0, zero)
	// Leave index 2 as the buffer's default blank cell.

	img, err := rasterizeGrid(buf)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("expected non-empty image")
	}
}

func TestRasterizeGridPaintsBackgroundColor(t *testing.T) {
	styles := map[int]cellbuf.Style{
		0: {Bg: ansi.RGBColor{R: 0xff, G: 0x00, B: 0x00}},
	}
	// A space glyph is never drawn (see drawCell's " " guard), so the sampled
	// pixel reflects the background fill alone, not a foreground glyph.
	buf := styledBuffer(t, "  ", styles)

	img, err := rasterizeGrid(buf)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}
	cell := measureCell(faces.regular)

	// Sample a pixel well inside the first cell's painted rectangle.
	x := imagePadding + cell.width/2
	y := imagePadding + cell.height/2
	r, g, b, _ := img.At(x, y).RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	r8 := uint8(r >> eightBitShift & byteMask)
	g8 := uint8(g >> eightBitShift & byteMask)
	b8 := uint8(b >> eightBitShift & byteMask)
	if r8 < 0x80 || g8 > 0x40 || b8 > 0x40 {
		t.Fatalf("background pixel = (%d,%d,%d), want red-ish", r8, g8, b8)
	}
}

func TestRasterizeGridUnderlineDrawsLine(t *testing.T) {
	style := cellbuf.Style{}
	style.Underline(true)
	buf := styledBuffer(t, "X", map[int]cellbuf.Style{0: style})

	img, err := rasterizeGrid(buf)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}
	cell := measureCell(faces.regular)
	yy := imagePadding + cell.ascent + 2

	r, g, b, a := img.At(imagePadding, yy).RGBA()
	fr, fg, fb, fa := imageForeground.RGBA()
	if r != fr || g != fg || b != fb || a != fa {
		t.Fatalf("underline pixel = (%d,%d,%d,%d), want foreground color", r, g, b, a)
	}
}

func TestRasterizeGridStrikethroughDrawsLine(t *testing.T) {
	style := cellbuf.Style{}
	style.Strikethrough(true)
	buf := styledBuffer(t, "X", map[int]cellbuf.Style{0: style})

	img, err := rasterizeGrid(buf)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}
	cell := measureCell(faces.regular)
	yy := imagePadding + cell.height/2

	r, g, b, a := img.At(imagePadding, yy).RGBA()
	fr, fg, fb, fa := imageForeground.RGBA()
	if r != fr || g != fg || b != fb || a != fa {
		t.Fatalf("strikethrough pixel = (%d,%d,%d,%d), want foreground color", r, g, b, a)
	}
}

func TestRenderPNGReturnsErrorOnMissingInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.png")
	err := RenderPNG(filepath.Join(t.TempDir(), "does-not-exist.cast"), out)
	if err == nil {
		t.Fatal("expected error for missing input cast file")
	}
}

func TestRenderPNGReturnsErrorWhenOutputCannotBeCreated(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "hi\n")
	// A parent directory that does not exist makes os.Create fail; RenderPNG
	// (unlike Render's prepareRenderOutput) does not create parent dirs.
	out := filepath.Join(t.TempDir(), "missing-parent", "out.png")
	if err := RenderPNG(cast, out); err == nil {
		t.Fatal("expected error when output parent directory is missing")
	}
}

func TestRenderJPEGReturnsErrorOnMissingInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.jpg")
	err := RenderJPEG(filepath.Join(t.TempDir(), "does-not-exist.cast"), out)
	if err == nil {
		t.Fatal("expected error for missing input cast file")
	}
}

func TestRenderJPEGReturnsErrorWhenOutputCannotBeCreated(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "hi\n")
	out := filepath.Join(t.TempDir(), "missing-parent", "out.jpg")
	if err := RenderJPEG(cast, out); err == nil {
		t.Fatal("expected error when output parent directory is missing")
	}
}

func TestRenderPNGDecodesWithExpectedBackgroundRegion(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "\x1b[41mred\x1b[0m\n")
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

	// Sample a pixel inside the first character cell, which should carry the
	// red SGR background rather than the default terminal background.
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}
	cell := measureCell(faces.regular)
	x := imagePadding + cell.width/2
	y := imagePadding + cell.height/2
	r, g, b, _ := img.At(x, y).RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	r8 := uint8(r >> eightBitShift & byteMask)
	g8 := uint8(g >> eightBitShift & byteMask)
	b8 := uint8(b >> eightBitShift & byteMask)
	bgR, bgG, bgB, _ := imageBackground.RGBA()
	bgR8 := uint8(bgR >> eightBitShift & byteMask)
	bgG8 := uint8(bgG >> eightBitShift & byteMask)
	bgB8 := uint8(bgB >> eightBitShift & byteMask)
	if r8 == bgR8 && g8 == bgG8 && b8 == bgB8 {
		t.Fatalf("pixel = (%d,%d,%d), still shows canvas background, want SGR red background", r8, g8, b8)
	}
}

func TestRenderJPEGDecodesNonTrivialImage(t *testing.T) {
	cast := writeTestCast(t, 10, 2, "\x1b[1;3mhi\x1b[0m\n")
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
	if img.Bounds().Dx() <= 2*imagePadding || img.Bounds().Dy() <= 2*imagePadding {
		t.Fatalf("image too small: %v", img.Bounds())
	}
}

func TestDrawCellSkipsGlyphForBlankText(t *testing.T) {
	// A cell containing only a space should still allow background painting
	// (covered separately) without attempting to draw a glyph.
	buf := cellbuf.NewBuffer(1, 1)
	cell := cellbuf.NewCell(' ')
	cell.Style.Bg = ansi.RGBColor{R: 0x00, G: 0xff, B: 0x00}
	buf.SetCell(0, 0, cell)

	img, err := rasterizeGrid(buf)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := loadGlyphFaces()
	if err != nil {
		t.Fatal(err)
	}
	m := measureCell(faces.regular)
	x := imagePadding + m.width/2
	y := imagePadding + m.height/2
	r, g, b, _ := img.At(x, y).RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	r8 := uint8(r >> eightBitShift & byteMask)
	g8 := uint8(g >> eightBitShift & byteMask)
	b8 := uint8(b >> eightBitShift & byteMask)
	if g8 < 0x80 || r8 > 0x40 || b8 > 0x40 {
		t.Fatalf("background pixel for blank cell = (%d,%d,%d), want green-ish", r8, g8, b8)
	}
}

// TestImageAtmosBlueOverrideAppliesToCellColors verifies the ANSI blue
// override is honored when resolving cell paint colors (not just the raw
// imageColor helper already covered in render_static_test.go).
func TestImageAtmosBlueOverrideAppliesToCellColors(t *testing.T) {
	style := cellbuf.Style{Fg: ansi.Blue}
	fg, _ := cellColors(&style)
	if fg != imageAtmosBlue {
		t.Fatalf("fg = %#v, want imageAtmosBlue", fg)
	}
}
