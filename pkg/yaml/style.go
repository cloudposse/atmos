package yaml

import (
	"bytes"
	"strconv"

	editorconfig "github.com/editorconfig/editorconfig-core-go/v2"
)

const (
	lineEndingLF   = "lf"
	lineEndingCRLF = "crlf"
)

type editorConfigStyle struct {
	indent                 int
	endOfLine              string
	insertFinalNewline     *bool
	trimTrailingWhitespace bool
}

type editOptions struct {
	indent int
}

type fileMutationMode int

const (
	fileMutationPreserve fileMutationMode = iota
	fileMutationFormat
)

func defaultEditOptions(content []byte) editOptions {
	return editOptions{indent: detectIndent(content)}
}

func editOptionsForFile(content []byte, style editorConfigStyle, mode fileMutationMode) editOptions {
	if mode == fileMutationFormat {
		if style.indent > 0 {
			return editOptions{indent: style.indent}
		}
		return defaultEditOptions(content)
	}

	if indent, ok := detectIndentWidth(content); ok {
		return editOptions{indent: indent}
	}
	if style.indent > 0 {
		return editOptions{indent: style.indent}
	}
	return defaultEditOptions(content)
}

func resolveEditorConfigStyle(filePath string) editorConfigStyle {
	def, err := editorconfig.GetDefinitionForFilename(filePath)
	if err != nil || def == nil {
		return editorConfigStyle{}
	}

	style := editorConfigStyle{
		endOfLine:          normalizeEditorConfigLineEnding(def.EndOfLine),
		insertFinalNewline: def.InsertFinalNewline,
	}
	if def.TrimTrailingWhitespace != nil && *def.TrimTrailingWhitespace {
		style.trimTrailingWhitespace = true
	}
	if indent, err := strconv.Atoi(def.IndentSize); err == nil && indent > 0 {
		style.indent = indent
	}
	return style
}

func normalizeEditorConfigLineEnding(endOfLine string) string {
	switch endOfLine {
	case editorconfig.EndOfLineLf:
		return lineEndingLF
	case editorconfig.EndOfLineCrLf:
		return lineEndingCRLF
	default:
		return ""
	}
}

func applyFileStyle(original, out []byte, style editorConfigStyle, mode fileMutationMode) []byte {
	if mode == fileMutationFormat && style.trimTrailingWhitespace {
		out = trimTrailingWhitespace(out)
	}
	if finalNewline := targetFinalNewline(original, style, mode); finalNewline != nil {
		if *finalNewline {
			out = ensureFinalNewline(out)
		} else {
			out = removeFinalNewline(out)
		}
	}

	return applyLineEndings(out, targetLineEnding(original, style, mode))
}

func targetLineEnding(original []byte, style editorConfigStyle, mode fileMutationMode) string {
	if mode == fileMutationFormat && style.endOfLine != "" {
		return style.endOfLine
	}
	if detected := detectLineEnding(original); detected != "" {
		return detected
	}
	return style.endOfLine
}

func targetFinalNewline(original []byte, style editorConfigStyle, mode fileMutationMode) *bool {
	if mode == fileMutationFormat {
		return style.insertFinalNewline
	}
	if len(original) == 0 {
		return style.insertFinalNewline
	}

	hasFinalNewline := bytes.HasSuffix(original, lfBytes) || bytes.HasSuffix(original, []byte("\r"))
	return &hasFinalNewline
}

func detectLineEnding(content []byte) string {
	lfCount := bytes.Count(content, lfBytes)
	if lfCount == 0 {
		return ""
	}
	switch crlfCount := bytes.Count(content, crlfBytes); crlfCount {
	case lfCount:
		return lineEndingCRLF
	case 0:
		return lineEndingLF
	default:
		return ""
	}
}

func applyLineEndings(out []byte, endOfLine string) []byte {
	normalized := bytes.ReplaceAll(out, crlfBytes, lfBytes)
	if endOfLine == lineEndingCRLF {
		return bytes.ReplaceAll(normalized, lfBytes, crlfBytes)
	}
	return normalized
}

func ensureFinalNewline(out []byte) []byte {
	if len(out) == 0 || bytes.HasSuffix(out, lfBytes) || bytes.HasSuffix(out, []byte("\r")) {
		return out
	}
	return append(out, '\n')
}

func removeFinalNewline(out []byte) []byte {
	switch {
	case bytes.HasSuffix(out, crlfBytes):
		return out[:len(out)-len(crlfBytes)]
	case bytes.HasSuffix(out, lfBytes), bytes.HasSuffix(out, []byte("\r")):
		return out[:len(out)-1]
	default:
		return out
	}
}

func trimTrailingWhitespace(out []byte) []byte {
	normalized := bytes.ReplaceAll(out, crlfBytes, lfBytes)
	lines := bytes.SplitAfter(normalized, lfBytes)
	for i, line := range lines {
		hasNewline := bytes.HasSuffix(line, lfBytes)
		if hasNewline {
			line = line[:len(line)-1]
		}
		line = bytes.TrimRight(line, " \t")
		if hasNewline {
			line = append(line, '\n')
		}
		lines[i] = line
	}
	return bytes.Join(lines, nil)
}
