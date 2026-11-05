package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const minAttributeContentWidth = 24

// formattedAttributeChange precomputes values once, before choosing their layout.
type formattedAttributeChange struct {
	change *AttributeChange
	before attributeValue
	after  attributeValue
	oldVal string
	newVal string
}

// formatOneAttributeChange resolves placeholders before formatting visible values.
func formatOneAttributeChange(change *AttributeChange, config *RenderConfig) formattedAttributeChange {
	fc := formattedAttributeChange{change: change}
	if change.Sensitive {
		fc.before = plainAttributeValue("(sensitive)")
		fc.after = plainAttributeValue("(sensitive)")
	} else {
		fc.before = formatAttributeValue(change.Before, config)
		if !change.Unknown {
			fc.after = formatAttributeValue(change.After, config)
		}
	}
	if change.Unknown {
		fc.after = plainAttributeValue("(known after apply)")
	}
	fc.oldVal = inlineAttributeValue(fc.before)
	fc.newVal = inlineAttributeValue(fc.after)
	// Retain quoted old scalar strings, without the former 40-byte truncation.
	if text, ok := change.Before.(string); ok && !change.Sensitive && !fc.before.isBlock() {
		fc.oldVal = fmt.Sprintf("%q", text)
	}
	return fc
}

// inlineAttributeValue distinguishes missing and empty values in arrow columns.
func inlineAttributeValue(value attributeValue) string {
	if len(value.lines) == 0 {
		return "(none)"
	}
	if value.isBlock() {
		return ""
	}
	if value.lines[0] == "" {
		return `""`
	}
	return value.lines[0]
}

// precomputeAttributeFormatting measures terminal cells, rather than bytes or runes.
func precomputeAttributeFormatting(changes []*AttributeChange, config *RenderConfig) (formatted []formattedAttributeChange, maxKeyWidth, maxOldValWidth int) {
	for _, change := range changes {
		fc := formatOneAttributeChange(change, config)
		maxKeyWidth = max(maxKeyWidth, ansi.StringWidth(change.Key))
		maxOldValWidth = max(maxOldValWidth, ansi.StringWidth(fc.oldVal))
		formatted = append(formatted, fc)
	}
	return formatted, maxKeyWidth, maxOldValWidth
}

// renderOneAttributeChange uses arrow alignment for single-line values, blocks beneath
// the header for multiline additions, and diffs for structured or multiline updates.
func renderOneAttributeChange(b *strings.Builder, fc *formattedAttributeChange, ctx attrRenderContext, widths attributeWidths) {
	info := &attrStyleInfo{
		KeyStyle: attributeKeyStyle(fc.change, ctx.Config), Annotation: forcesReplacementAnnotation(fc.change),
	}
	if fc.change.Before != nil && (fc.before.isBlock() || fc.after.isBlock()) {
		renderAttributeDiff(b, fc, ctx, info)
		return
	}
	if len(fc.after.lines) > 1 {
		renderCompactAttribute(b, fc, ctx, info, widths)
		return
	}
	key := padAttributeColumn(fc.change.Key, widths.Key)
	old := padAttributeColumn(fc.oldVal, widths.OldVal)
	prefix := ctx.Indent + ctx.Bar + info.KeyStyle.Render(key) + spaceChar +
		ctx.Config.DimStyle.Render(old+"  →  ")
	if ctx.Config.Width-ansi.StringWidth(prefix) < minAttributeContentWidth {
		renderCompactAttribute(b, fc, ctx, info, widths)
		return
	}
	continuation := ctx.Indent + ctx.Bar + strings.Repeat(spaceChar, ansi.StringWidth(prefix)-ansi.StringWidth(ctx.Indent+ctx.Bar))
	lines := fc.after.displayLines(ctx.Config)
	if !fc.after.isBlock() {
		lines = []string{fc.newVal}
	}
	renderArrowValue(b, lines, prefix, continuation, ctx.Config.Width)
	renderAttributeAnnotation(b, info.Annotation, ctx)
}

// padAttributeColumn aligns columns using visible terminal cells.
func padAttributeColumn(text string, width int) string {
	return text + strings.Repeat(spaceChar, max(0, width-ansi.StringWidth(text)))
}

// renderArrowValue repeats the continuation gutter for every wrapped logical line.
func renderArrowValue(b *strings.Builder, lines []string, prefix, continuation string, width int) {
	for _, line := range lines {
		writeWrappedAttributeLine(b, line, prefix, continuation, width)
		prefix = continuation
	}
}

// renderCompactAttribute places multiline additions beneath their header at any width,
// and single-line additions there when the arrow's column leaves too little room.
// Headers retain the shared key/old-value columns whenever the header itself fits.
// Oversized old/new comparisons become full, untruncated diffs.
func renderCompactAttribute(b *strings.Builder, fc *formattedAttributeChange, ctx attrRenderContext, info *attrStyleInfo, widths attributeWidths) {
	if fc.change.Before != nil || fc.change.Sensitive {
		renderAttributeDiff(b, fc, ctx, info)
		return
	}
	prefix := ctx.Indent + ctx.Bar
	key := padAttributeColumn(fc.change.Key, widths.Key)
	old := padAttributeColumn(fc.oldVal, widths.OldVal)
	header := info.KeyStyle.Render(key) + spaceChar + ctx.Config.DimStyle.Render(old+"  →")
	if ansi.StringWidth(prefix+header) > ctx.Config.Width {
		header = info.KeyStyle.Render(fc.change.Key) + ctx.Config.DimStyle.Render(" (none)  →")
	}
	writeWrappedAttributeLine(b, header, prefix, prefix, ctx.Config.Width)
	indent := prefix + twoSpaceIndent
	lines := fc.after.displayLines(ctx.Config)
	if !fc.after.isBlock() {
		lines = []string{fc.newVal}
	}
	renderArrowValue(b, lines, indent, indent, ctx.Config.Width)
	renderAttributeAnnotation(b, info.Annotation, ctx)
}

// renderAttributeAnnotation wraps replacement notes within the attribute gutter.
func renderAttributeAnnotation(b *strings.Builder, annotation string, ctx attrRenderContext) {
	if annotation != "" {
		prefix := ctx.Indent + ctx.Bar
		writeWrappedAttributeLine(b, strings.TrimPrefix(annotation, spaceChar), prefix, prefix, ctx.Config.Width)
	}
}

// renderAttributeDiff puts the change header above its indented line diff.
func renderAttributeDiff(b *strings.Builder, fc *formattedAttributeChange, ctx attrRenderContext, info *attrStyleInfo) {
	prefix := ctx.Indent + ctx.Bar
	header := info.KeyStyle.Render(fc.change.Key)
	if ansi.StringWidth(prefix+header+info.Annotation) <= ctx.Config.Width {
		writeWrappedAttributeLine(b, header+info.Annotation, prefix, prefix, ctx.Config.Width)
	} else {
		writeWrappedAttributeLine(b, header, prefix, prefix, ctx.Config.Width)
		renderAttributeAnnotation(b, info.Annotation, ctx)
	}
	ctx.Indent = prefix + twoSpaceIndent
	ctx.Bar = ""
	before, after := attributeDiffValues(fc)
	renderValueDiff(b, before, after, ctx)
}

// attributeDiffValues falls back to literal strings when document normalization
// would hide a formatting-only change, retaining document highlighting and line
// limits without exposing protected values.
func attributeDiffValues(fc *formattedAttributeChange) (attributeValue, attributeValue) {
	before, beforeString := fc.change.Before.(string)
	after, afterString := fc.change.After.(string)
	if !fc.change.Sensitive && !fc.change.Unknown && beforeString && afterString && before != after &&
		fc.before.format != "" && slices.Equal(fc.before.lines, fc.after.lines) {
		return attributeValue{lines: strings.Split(before, newlineStr), format: fc.before.format},
			attributeValue{lines: strings.Split(after, newlineStr), format: fc.after.format}
	}
	return fc.before, fc.after
}

// renderValueDiff compares uncolored, unwrapped lines and groups removals before additions.
// The corresponding highlighted lines are used only when emitting the physical rows.
func renderValueDiff(b *strings.Builder, before, after attributeValue, ctx attrRenderContext) {
	beforeLines, afterLines := before.diffLines(ctx.Config), after.diffLines(ctx.Config)
	beforeDisplay, afterDisplay := before.displayLines(ctx.Config), after.displayLines(ctx.Config)
	changedOmission := -1
	if slices.Equal(beforeLines, afterLines) && !slices.Equal(before.lines, after.lines) {
		// Equal collapsed views can conceal a change in the omitted middle.
		changedOmission = max(1, ctx.Config.MaxLines*truncHeadRatioNum/truncRatioDenom)
	}
	cursor := diffCursor{}
	for cursor.I < len(beforeLines) || cursor.J < len(afterLines) {
		if linesMatch(beforeLines, afterLines, cursor.I, cursor.J) {
			if cursor.I == changedOmission {
				renderAttributeDiffLine(b, beforeDisplay[cursor.I], ctx.Config.DeleteStyle.Render("-"), ctx)
				renderAttributeDiffLine(b, afterDisplay[cursor.J], ctx.Config.CreateStyle.Render("+"), ctx)
			} else {
				renderAttributeDiffLine(b, beforeDisplay[cursor.I], spaceChar, ctx)
			}
			cursor.I++
			cursor.J++
			continue
		}
		_, _, next := collectChanges(beforeLines, afterLines, cursor)
		for _, line := range beforeDisplay[cursor.I:next.I] {
			renderAttributeDiffLine(b, line, ctx.Config.DeleteStyle.Render("-"), ctx)
		}
		for _, line := range afterDisplay[cursor.J:next.J] {
			renderAttributeDiffLine(b, line, ctx.Config.CreateStyle.Render("+"), ctx)
		}
		cursor = next
	}
}

// renderAttributeDiffLine wraps a value with its change marker on the first row.
func renderAttributeDiffLine(b *strings.Builder, line, symbol string, ctx attrRenderContext) {
	prefix := ctx.Indent + symbol + spaceChar
	writeWrappedAttributeLine(b, line, prefix, ctx.Indent+twoSpaceIndent, ctx.Config.Width)
}

// writeWrappedAttributeLine preserves all content, ANSI styles, and grapheme clusters.
// Cut reapplies active styles to each physical segment, so gutter styles cannot erase
// the color of a token that spans a wrap. Every segment includes the original gutter.
func writeWrappedAttributeLine(b *strings.Builder, line, prefix, continuation string, width int) {
	available := max(1, width-max(ansi.StringWidth(prefix), ansi.StringWidth(continuation)))
	wrapped := ansi.Hardwrap(ansi.Strip(line), available, true)
	offset := 0
	for _, part := range strings.Split(wrapped, newlineStr) {
		end := offset + ansi.StringWidth(part)
		fmt.Fprintf(b, "%s%s\n", prefix, ansi.Cut(line, offset, end))
		offset = end
		prefix = continuation
	}
}
