package literalsource

import (
	"strconv"
	"strings"
)

const scalarWhitespace = " \t\r\n"

// Raw template files need not be valid YAML. Shield scalar source without parsing
// the surrounding template, and restore the original spelling after rendering.
// Quoted and block scalars are scanned as units, so tag text inside them is data.
func protectTemplateScalars(input string) (string, func(string) string) {
	var output strings.Builder
	replacements := []string{}
	last := 0
	flowDepth := 0
	for i := 0; i < len(input); {
		if end := untaggedEnd(input, i); end > i {
			i = end
			continue
		}
		if literalTagAt(input, i) {
			start := i + len("!literal")
			for start < len(input) && strings.ContainsRune(scalarWhitespace, rune(input[start])) {
				start++
			}
			end := scalarEnd(input, start, flowDepth > 0)
			if end > start {
				token := Encode("literal-source:" + strconv.Itoa(len(replacements)))
				for strings.Contains(input, token) {
					token = Encode(token)
				}
				quoted := strconv.Quote(token)
				// Hide the tag as well as its scalar so other source protectors
				// cannot encode our temporary placeholder as a second literal.
				output.WriteString(input[last:i])
				output.WriteString(quoted)
				replacements = append(replacements, quoted, input[i:end])
				last, i = end, end
				continue
			}
		}
		flowDepth = updateFlowDepth(input, i, flowDepth)
		i++
	}
	output.WriteString(input[last:])
	return output.String(), strings.NewReplacer(replacements...).Replace
}

func updateFlowDepth(input string, position, depth int) int {
	switch input[position] {
	case '[', '{':
		if depth > 0 && scalarBoundary(input, position) || flowCollectionStart(input, position) {
			return depth + 1
		}
	case ']', '}':
		if depth > 0 {
			return depth - 1
		}
	}
	return depth
}

// Opening brackets in an ordinary block plain scalar are data. A collection
// starts at a root/sequence value or after a mapping key, not after arbitrary
// scalar text (including a comma). Existing flow collections admit nested ones.
func flowCollectionStart(input string, position int) bool {
	line := strings.LastIndexByte(input[:position], '\n') + 1
	prefix := strings.TrimSpace(input[line:position])
	properties := strings.Fields(prefix)
	for len(properties) > 0 {
		last := properties[len(properties)-1]
		if !strings.HasPrefix(last, "&") && !strings.HasPrefix(last, "!") {
			break
		}
		properties = properties[:len(properties)-1]
	}
	prefix = strings.Join(properties, " ")
	return prefix == "" || strings.HasSuffix(prefix, ":") || strings.Trim(prefix, "- \t") == ""
}

func literalTagAt(input string, i int) bool {
	const tag = "!literal"
	return strings.HasPrefix(input[i:], tag) && scalarBoundary(input, i) &&
		(i+len(tag) == len(input) || strings.ContainsRune(scalarWhitespace, rune(input[i+len(tag)])))
}

// Skip ordinary YAML scalar and comment bodies before searching for a native tag.
func untaggedEnd(input string, i int) int {
	if !scalarBoundary(input, i) {
		return i
	}
	switch input[i] {
	case '#':
		return lineEnd(input, i)
	case '\'', '"':
		return quotedEnd(input, i)
	case '|', '>':
		if blockHeader(input[i:lineEnd(input, i)]) {
			return blockEnd(input, i)
		}
	}
	return i
}

func scalarBoundary(input string, i int) bool {
	return i == 0 || strings.ContainsRune(" \t\r\n:[{,?-", rune(input[i-1]))
}

func lineEnd(input string, start int) int {
	if end := strings.IndexByte(input[start:], '\n'); end >= 0 {
		return start + end
	}
	return len(input)
}

func quotedEnd(input string, start int) int {
	quote := input[start]
	for i := start + 1; i < len(input); i++ {
		if quote == '"' && input[i] == '\\' {
			i++
			continue
		}
		if input[i] != quote {
			continue
		}
		if quote == '\'' && i+1 < len(input) && input[i+1] == quote {
			i++
			continue
		}
		return i + 1
	}
	return len(input)
}

func blockHeader(header string) bool {
	for _, char := range header[1:] {
		if char == '#' {
			return true
		}
		if !strings.ContainsRune("+-123456789 \t\r", char) {
			return false
		}
	}
	return true
}

func indentation(input string, position int) int {
	start := strings.LastIndexByte(input[:position], '\n') + 1
	end := start
	for end < len(input) && input[end] == ' ' {
		end++
	}
	return end - start
}

// Keep the separating newline outside the replacement; include empty trailing
// lines so YAML block chomping retains the exact original body on restoration.
func blockEnd(input string, start int) int {
	indent := indentation(input, start)
	end := lineEnd(input, start)
	for next := end + 1; next < len(input); {
		line := lineEnd(input, next)
		if strings.TrimSpace(input[next:line]) != "" && indentation(input, next) <= indent {
			break
		}
		end, next = line, line+1
	}
	return end
}

func anchorValueStart(input string, start int) int {
	for start < len(input) && !strings.ContainsRune(scalarWhitespace, rune(input[start])) {
		start++
	}
	for start < len(input) && strings.ContainsRune(scalarWhitespace, rune(input[start])) {
		start++
	}
	return start
}

func scalarEnd(input string, start int, inFlow bool) int {
	if start >= len(input) {
		return start
	}
	switch input[start] {
	case '&':
		return scalarEnd(input, anchorValueStart(input, start), inFlow)
	case '\'', '"':
		return quotedEnd(input, start)
	case '|', '>':
		return blockEnd(input, start)
	case '[', '{', '\n', '\r':
		// Collections retain their existing behavior.
		return start
	}
	return plainScalarEnd(input, start, inFlow)
}

func plainScalarEnd(input string, start int, inFlow bool) int {
	end := lineEnd(input, start)
	for i := start; i < end; i++ {
		if input[i] == '#' && scalarBoundary(input, i) || inFlow && strings.ContainsRune(",]}", rune(input[i])) {
			return i
		}
	}
	return blockEnd(input, start)
}
