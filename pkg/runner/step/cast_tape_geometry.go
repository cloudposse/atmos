package step

import (
	"math"
	"strconv"

	"github.com/cloudposse/atmos/pkg/asciicast"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VHS sizes its terminal in pixels (`Set Width`/`Set Height`) while Atmos casts
// are sized in character cells. This file converts one to the other using the
// same inputs VHS uses to lay out its canvas.
const (
	// Defaults of the VHS canvas (https://github.com/charmbracelet/vhs). They apply when a tape
	// sets Width or Height without also setting the matching font and padding keys.
	vhsDefaultFontSize      = 22.0
	vhsDefaultPadding       = 60.0
	vhsDefaultLineHeight    = 1.0
	vhsDefaultLetterSpacing = 0.0

	// The advance width of a monospace glyph as a fraction of the font size.
	vhsGlyphWidthRatio = 0.6

	// The smallest terminal a tape-derived size is clamped to, so a tiny or malformed pixel size
	// can never produce an unusable recording.
	tapeMinColumns = 20
	tapeMinRows    = 5

	// Absorbs floating-point error so an exact fit (for example 1380 / 8.4) does not floor one
	// cell short.
	tapeGeometryEpsilon = 1e-9

	// The bit size Set numbers are parsed with.
	tapeFloatBitSize = 64

	// Set keys that feed the pixel-to-cell conversion.
	tapeKeyWidth         = "Width"
	tapeKeyHeight        = "Height"
	tapeKeyFontSize      = "FontSize"
	tapeKeyPadding       = "Padding"
	tapeKeyLineHeight    = "LineHeight"
	tapeKeyLetterSpacing = "LetterSpacing"
)

// tapeGeometry accumulates the Set directives that determine the terminal
// size. Every key is collected first and applied once at the end, so the
// result does not depend on the order the keys appear in the tape.
type tapeGeometry struct {
	widthPx, heightPx         float64
	hasWidth, hasHeight       bool
	fontSize, padding         float64
	lineHeight, letterSpacing float64
	stepName                  string
}

func newTapeGeometry(stepName string) *tapeGeometry {
	return &tapeGeometry{
		fontSize:      vhsDefaultFontSize,
		padding:       vhsDefaultPadding,
		lineHeight:    vhsDefaultLineHeight,
		letterSpacing: vhsDefaultLetterSpacing,
		stepName:      stepName,
	}
}

// isTapeGeometryKey reports whether a Set key feeds the pixel-to-cell conversion.
func isTapeGeometryKey(key string) bool {
	switch key {
	case tapeKeyWidth, tapeKeyHeight, tapeKeyFontSize, tapeKeyPadding, tapeKeyLineHeight, tapeKeyLetterSpacing:
		return true
	default:
		return false
	}
}

// record stores one geometry Set directive. A value that is not a number is
// ignored with a warning, leaving the VHS default (or the previous value) in
// place.
func (g *tapeGeometry) record(d *asciicast.TapeDirective) {
	value, err := strconv.ParseFloat(d.Value, tapeFloatBitSize)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		log.Warn("tape: Set directive has a non-numeric value, ignoring",
			"key", d.Key, "value", d.Value, "step", g.stepName, "file", d.File, "line", d.Line)
		return
	}
	switch d.Key {
	case tapeKeyWidth:
		g.widthPx, g.hasWidth = value, true
	case tapeKeyHeight:
		g.heightPx, g.hasHeight = value, true
	case tapeKeyFontSize:
		g.fontSize = value
	case tapeKeyPadding:
		g.padding = value
	case tapeKeyLineHeight:
		g.lineHeight = value
	case tapeKeyLetterSpacing:
		g.letterSpacing = value
	}
}

// columns converts the pixel width into character columns.
func (g *tapeGeometry) columns() (int, bool) {
	cellWidth := g.fontSize*vhsGlyphWidthRatio + g.letterSpacing
	if cellWidth <= 0 {
		return 0, false
	}
	return g.clampCells(tapeKeyWidth, g.widthPx, (g.widthPx-2*g.padding)/cellWidth, tapeMinColumns), true
}

// rows converts the pixel height into character rows.
func (g *tapeGeometry) rows() (int, bool) {
	cellHeight := g.fontSize * g.lineHeight
	if cellHeight <= 0 {
		return 0, false
	}
	return g.clampCells(tapeKeyHeight, g.heightPx, (g.heightPx-2*g.padding)/cellHeight, tapeMinRows), true
}

// clampCells floors a derived cell count and raises it to minimum. Hitting the
// minimum usually means the tape's value was written as a cell count rather
// than VHS pixels (e.g. `Set Width 80`), so it warns instead of clamping silently.
func (g *tapeGeometry) clampCells(key string, pixels, cells float64, minimum int) int {
	n := int(math.Floor(cells + tapeGeometryEpsilon))
	if n >= minimum {
		return n
	}
	log.Warn("tape: Set value is in VHS pixels and is too small for a usable terminal, clamping",
		"key", key, "pixels", pixels, "derived", n, "clamped_to", minimum, "step", g.stepName,
		"hint", "set the cast step's width/height fields to size the terminal in cells")
	return minimum
}

// apply fills step.Width and step.Height from the collected pixel sizes. An
// explicit step width or height always wins, and only the dimensions the tape
// actually set are derived.
func (g *tapeGeometry) apply(step *schema.WorkflowStep) {
	if g.hasWidth && step.Width <= 0 {
		if cols, ok := g.columns(); ok {
			step.Width = cols
		} else {
			g.warnUnusableMetrics(step, "FontSize and LetterSpacing")
		}
	}
	if g.hasHeight && step.Height <= 0 {
		if rows, ok := g.rows(); ok {
			step.Height = rows
		} else {
			g.warnUnusableMetrics(step, "FontSize and LineHeight")
		}
	}
}

func (g *tapeGeometry) warnUnusableMetrics(step *schema.WorkflowStep, keys string) {
	log.Warn("tape: cannot convert pixel size to terminal cells, ignoring",
		"reason", "non-positive cell size", "keys", keys, "step", step.Name)
}
