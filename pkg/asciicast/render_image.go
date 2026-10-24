package asciicast

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	imageFontSize = 14.0
	imageDPI      = 72.0
	imagePadding  = 16
	jpegQuality   = 90
	faintDim      = 0.6
)

// Terminal-window palette for rasterized screengrabs.
var (
	imageBackground = color.RGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}
	imageForeground = color.RGBA{R: 0xd0, G: 0xd0, B: 0xd0, A: 0xff}
	imageAtmosBlue  = color.RGBA{R: 0x00, G: 0x5f, B: 0x87, A: 0xff}
)

// RenderPNG writes the final terminal content of a cast file as a PNG image.
func RenderPNG(input, output string) error {
	defer perf.Track(nil, "asciicast.RenderPNG")()

	img, err := castImage(input)
	if err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return png.Encode(file, img)
}

// RenderJPEG writes the final terminal content of a cast file as a JPEG image.
func RenderJPEG(input, output string) error {
	defer perf.Track(nil, "asciicast.RenderJPEG")()

	img, err := castImage(input)
	if err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return jpeg.Encode(file, img, &jpeg.Options{Quality: jpegQuality})
}

func castImage(input string) (image.Image, error) {
	grid, err := BuildGrid(input)
	if err != nil {
		return nil, err
	}
	return rasterizeGrid(grid)
}

// glyphFaces holds the four monospace font faces used to rasterize cells.
type glyphFaces struct {
	regular    font.Face
	bold       font.Face
	italic     font.Face
	boldItalic font.Face
}

func loadGlyphFaces() (*glyphFaces, error) {
	faces := &glyphFaces{}
	for _, entry := range []struct {
		ttf  []byte
		face *font.Face
	}{
		{gomono.TTF, &faces.regular},
		{gomonobold.TTF, &faces.bold},
		{gomonoitalic.TTF, &faces.italic},
		{gomonobolditalic.TTF, &faces.boldItalic},
	} {
		parsed, err := opentype.Parse(entry.ttf)
		if err != nil {
			return nil, fmt.Errorf("parse embedded font: %w", err)
		}
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
			Size:    imageFontSize,
			DPI:     imageDPI,
			Hinting: font.HintingFull,
		})
		if err != nil {
			return nil, fmt.Errorf("create font face: %w", err)
		}
		*entry.face = face
	}
	return faces, nil
}

func (f *glyphFaces) forStyle(s *cellbuf.Style) font.Face {
	bold := s.Attrs.Contains(cellbuf.BoldAttr)
	italic := s.Attrs.Contains(cellbuf.ItalicAttr)
	switch {
	case bold && italic:
		return f.boldItalic
	case bold:
		return f.bold
	case italic:
		return f.italic
	default:
		return f.regular
	}
}

// cellMetrics captures the pixel geometry of one terminal cell.
type cellMetrics struct {
	width   int
	height  int
	ascent  int
	advance fixed.Int26_6
}

func measureCell(face font.Face) cellMetrics {
	metrics := face.Metrics()
	advance, _ := face.GlyphAdvance('M')
	return cellMetrics{
		width:   advance.Ceil(),
		height:  metrics.Height.Ceil(),
		ascent:  metrics.Ascent.Ceil(),
		advance: advance,
	}
}

func rasterizeGrid(grid *cellbuf.Buffer) (image.Image, error) {
	faces, err := loadGlyphFaces()
	if err != nil {
		return nil, err
	}
	cell := measureCell(faces.regular)
	cols, rows := grid.Width(), grid.Height()
	img := image.NewRGBA(image.Rect(0, 0, cols*cell.width+2*imagePadding, rows*cell.height+2*imagePadding))
	draw.Draw(img, img.Bounds(), image.NewUniform(imageBackground), image.Point{}, draw.Src)

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := grid.Cell(x, y)
			if c == nil || c.Width == 0 {
				continue
			}
			origin := image.Pt(imagePadding+x*cell.width, imagePadding+y*cell.height)
			drawCell(img, faces, cell, origin, c)
		}
	}
	return img, nil
}

func drawCell(img *image.RGBA, faces *glyphFaces, cell cellMetrics, origin image.Point, c *cellbuf.Cell) {
	fg, bg := cellColors(&c.Style)
	if bg != nil {
		rect := image.Rect(origin.X, origin.Y, origin.X+cell.width*c.Width, origin.Y+cell.height)
		draw.Draw(img, rect, image.NewUniform(bg), image.Point{}, draw.Src)
	}
	if text := c.String(); text != " " && text != "" {
		drawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(fg),
			Face: faces.forStyle(&c.Style),
			Dot:  fixed.P(origin.X, origin.Y+cell.ascent),
		}
		drawer.DrawString(text)
	}
	drawCellDecorations(img, cell, origin, c, fg)
}

func drawCellDecorations(img *image.RGBA, cell cellMetrics, origin image.Point, c *cellbuf.Cell, fg color.Color) {
	lineWidth := cell.width * c.Width
	if c.Style.UlStyle != cellbuf.NoUnderline {
		yy := origin.Y + cell.ascent + 2
		draw.Draw(img, image.Rect(origin.X, yy, origin.X+lineWidth, yy+1), image.NewUniform(fg), image.Point{}, draw.Src)
	}
	if c.Style.Attrs.Contains(cellbuf.StrikethroughAttr) {
		yy := origin.Y + cell.height/2
		draw.Draw(img, image.Rect(origin.X, yy, origin.X+lineWidth, yy+1), image.NewUniform(fg), image.Point{}, draw.Src)
	}
}

// cellColors resolves the foreground and background paint colors for a cell,
// honoring reverse video and faint dimming. A nil background means "do not
// paint" (the canvas background shows through).
func cellColors(s *cellbuf.Style) (fg color.Color, bg color.Color) {
	fg = imageForeground
	if s.Fg != nil {
		fg = imageColor(s.Fg)
	}
	if s.Bg != nil {
		bg = imageColor(s.Bg)
	}
	if s.Attrs.Contains(cellbuf.ReverseAttr) {
		swapped := fg
		fg = imageBackground
		if bg != nil {
			fg = bg
		}
		bg = swapped
	}
	if s.Attrs.Contains(cellbuf.FaintAttr) {
		fg = dimColor(fg)
	}
	return fg, bg
}

// imageColor resolves an ANSI color for rasterization, applying the same
// Atmos blue override used by the HTML renderer.
func imageColor(c ansi.Color) color.Color {
	if basic, ok := c.(ansi.BasicColor); ok && (basic == ansi.Blue || basic == ansi.BrightBlue) {
		return imageAtmosBlue
	}
	return c
}

func dimColor(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	return color.RGBA{
		R: uint8(float64(r>>eightBitShift&byteMask) * faintDim),
		G: uint8(float64(g>>eightBitShift&byteMask) * faintDim),
		B: uint8(float64(b>>eightBitShift&byteMask) * faintDim),
		A: uint8(a >> eightBitShift & byteMask),
	}
}
