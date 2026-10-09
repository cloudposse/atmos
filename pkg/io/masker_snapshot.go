package io

import (
	"regexp"
	"sort"
	"strings"
)

const (
	// Literals of at least this length also match a whitespace-tolerant (YAML-folded) rendering.
	foldedLiteralMinLen = 32

	// The two flexSpan constants bound how far a whitespace-tolerant literal can stretch in the
	// input when indentation is inserted. They only size the streaming hold-back window; Mask
	// itself is not bounded by them.
	flexSpanBase     = 256
	flexSpanPerSpace = 128

	// Separator between the lines of a multiline literal.
	lineSeparator = "\n"
)

// literalEntry is one registered literal plus the regexes derived from it.
type literalEntry struct {
	text string

	// indented matches the literal after a serializer indented its continuation lines.
	// It is nil unless the literal spans several lines.
	indented *regexp.Regexp

	// folded matches the literal after a YAML emitter folded ordinary spaces into newlines.
	// It is nil unless the literal is long and contains spaces or tabs.
	folded *regexp.Regexp

	// canon is the literal with every whitespace byte removed. It is set only when indented or
	// folded is non-nil and is used to detect partially streamed whitespace-tolerant matches.
	canon string

	// span is the largest input length a whitespace-tolerant match can have.
	span int
}

// flexible reports whether the entry also matches whitespace-altered renderings.
func (e *literalEntry) flexible() bool {
	return e.indented != nil || e.folded != nil
}

// prefixProbe describes one way a registered literal can begin in a stream.
type prefixProbe struct {
	text  string
	canon bool // text is a whitespace-free form compared with whitespace skipped.
	span  int  // Longest input span that can still be a proper prefix.
}

// maskSnapshot is an immutable, precomputed view of the registered literals and patterns.
// Building it once per registration change (instead of once per Mask call) removes the
// per-call sort and regex compilation from the hot path.
type maskSnapshot struct {
	literals []literalEntry // Longest first.
	patterns []*regexp.Regexp

	// probes indexes prefix probes by their first byte for the streaming hold-back scan.
	probes    [256][]prefixProbe
	probeSpan int // Largest probe span; bounds the hold-back scan window.
}

// buildMaskSnapshot derives the snapshot from the registered literals and patterns.
func buildMaskSnapshot(literals map[string]bool, patterns []*regexp.Regexp) *maskSnapshot {
	snap := &maskSnapshot{
		literals: make([]literalEntry, 0, len(literals)),
		patterns: append([]*regexp.Regexp(nil), patterns...),
	}

	for literal := range literals {
		if literal == "" {
			continue
		}
		snap.literals = append(snap.literals, newLiteralEntry(literal))
	}

	// Longest first prevents shorter literals from being replaced before longer ones.
	// Ties break lexicographically so the order is deterministic across runs.
	sort.Slice(snap.literals, func(i, j int) bool {
		a, b := snap.literals[i].text, snap.literals[j].text
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a < b
	})

	for i := range snap.literals {
		snap.addProbes(&snap.literals[i])
	}
	return snap
}

// newLiteralEntry derives the whitespace-tolerant variants of a literal.
func newLiteralEntry(literal string) literalEntry {
	entry := literalEntry{text: literal}
	entry.indented = indentedMultilineRegexp(literal)
	entry.folded = foldedLiteralRegexp(literal)
	if entry.flexible() {
		spaces := 0
		var canon strings.Builder
		for i := 0; i < len(literal); i++ {
			if isFlexSpace(literal[i]) {
				spaces++
				continue
			}
			canon.WriteByte(literal[i])
		}
		entry.canon = canon.String()
		entry.span = len(literal) + flexSpanBase + flexSpanPerSpace*spaces
	}
	return entry
}

// addProbes registers how the entry can begin in a stream.
func (s *maskSnapshot) addProbes(e *literalEntry) {
	if len(e.text) > 1 {
		s.addProbe(e.text[0], prefixProbe{text: e.text, span: len(e.text) - 1})
	}
	if e.flexible() && len(e.canon) > 1 {
		s.addProbe(e.canon[0], prefixProbe{text: e.canon, canon: true, span: e.span})
	}
}

func (s *maskSnapshot) addProbe(first byte, probe prefixProbe) {
	s.probes[first] = append(s.probes[first], probe)
	if probe.span > s.probeSpan {
		s.probeSpan = probe.span
	}
}

// mask applies literals (longest first) and then regex patterns.
func (s *maskSnapshot) mask(input, replacement string) string {
	masked := input

	// Escape $ as $$ so the replacement is never interpreted as a backreference.
	quoted := strings.ReplaceAll(replacement, "$", "$$")

	for i := range s.literals {
		e := &s.literals[i]
		masked = strings.ReplaceAll(masked, e.text, replacement)
		if e.indented != nil {
			masked = e.indented.ReplaceAllString(masked, quoted)
		}
		if e.folded != nil {
			masked = e.folded.ReplaceAllString(masked, quoted)
		}
	}

	for _, pattern := range s.patterns {
		masked = pattern.ReplaceAllString(masked, quoted)
	}

	return masked
}

// holdbackLen returns how many trailing bytes of input a streaming writer must withhold so that
// emitting input[:len(input)-n] cannot split a registered secret across two writes.
func (s *maskSnapshot) holdbackLen(input string, lineBoundary bool) int {
	n := len(input)
	cut := n - s.partialSuffixLen(input)

	if lineBoundary && len(s.patterns) > 0 {
		// Regex patterns have no bounded prefix, so hold the entire unfinished line.
		// A fixed tail could drop a pattern's prefix and expose its secret suffix.
		boundary := strings.LastIndexAny(input, "\r\n") + 1
		cut = min(cut, boundary)
	}

	if cut < n {
		cut = s.avoidStraddle(input, cut)
	}
	return n - cut
}

// partialSuffixLen returns the length of the longest suffix of input that is a proper prefix of
// a registered literal (or of its whitespace-tolerant rendering). It returns 0 when there is none.
func (s *maskSnapshot) partialSuffixLen(input string) int {
	n := len(input)
	if s.probeSpan == 0 || n == 0 {
		return 0
	}

	for i := max(0, n-s.probeSpan); i < n; i++ {
		rest := input[i:]
		for _, probe := range s.probes[input[i]] {
			if probe.matchesPrefix(rest) {
				return len(rest)
			}
		}
	}
	return 0
}

// matchesPrefix reports whether rest is a proper prefix of the probe.
func (p *prefixProbe) matchesPrefix(rest string) bool {
	if len(rest) > p.span {
		return false
	}
	if !p.canon {
		return len(rest) < len(p.text) && strings.HasPrefix(p.text, rest)
	}

	k := 0
	for j := 0; j < len(rest); j++ {
		c := rest[j]
		if isFlexSpace(c) {
			continue
		}
		if k >= len(p.text) || p.text[k] != c {
			return false
		}
		k++
	}
	return k < len(p.text)
}

// avoidStraddle moves cut left until no complete literal occurrence crosses it.
func (s *maskSnapshot) avoidStraddle(input string, cut int) int {
	for moved := true; moved && cut > 0; {
		moved = false
		for i := range s.literals {
			e := &s.literals[i]
			if next, ok := e.straddleStart(input, cut); ok {
				cut, moved = next, true
			}
		}
	}
	return cut
}

// straddleStart returns the start of a complete occurrence of the entry that crosses cut.
func (e *literalEntry) straddleStart(input string, cut int) (int, bool) {
	if len(e.text) > 1 {
		from := max(0, cut-len(e.text)+1)
		if idx := strings.Index(input[from:], e.text); idx >= 0 && from+idx < cut {
			return from + idx, true
		}
	}

	if !e.flexible() {
		return 0, false
	}
	from := max(0, cut-e.span)
	for _, re := range []*regexp.Regexp{e.indented, e.folded} {
		if re == nil {
			continue
		}
		for _, loc := range re.FindAllStringIndex(input[from:], -1) {
			if from+loc[0] < cut && from+loc[1] > cut {
				return from + loc[0], true
			}
		}
	}
	return 0, false
}

// isFlexSpace reports whether b is whitespace that serializers may add, remove, or fold.
func isFlexSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// foldedLiteralRegexp returns a pattern that matches a long scalar value after a YAML emitter
// folds an ordinary space into a newline plus indentation. All non-whitespace bytes must still
// match exactly. It returns nil when the literal is not eligible.
func foldedLiteralRegexp(literal string) *regexp.Regexp {
	if len(literal) < foldedLiteralMinLen || !strings.ContainsAny(literal, " \t") {
		return nil
	}

	var pattern strings.Builder
	for _, char := range literal {
		if char == ' ' || char == '\t' {
			pattern.WriteString(`(?:[ \t]|\r?\n[ \t]+)`)
		} else {
			pattern.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	return regexp.MustCompile(pattern.String())
}

// indentedMultilineRegexp returns a pattern that matches a registered multiline value after
// serializers such as YAML have indented its continuation lines. The payload lines must still
// match exactly; only indentation introduced after a newline is ignored. It returns nil when the
// literal is a single line.
func indentedMultilineRegexp(literal string) *regexp.Regexp {
	normalized := strings.ReplaceAll(literal, "\r\n", lineSeparator)
	normalized = strings.TrimRight(normalized, lineSeparator)
	if !strings.Contains(normalized, lineSeparator) {
		return nil
	}

	lines := strings.Split(normalized, lineSeparator)
	var pattern strings.Builder
	for i, line := range lines {
		if i > 0 {
			pattern.WriteString(`\r?\n[ \t]*`)
		}
		pattern.WriteString(regexp.QuoteMeta(line))
	}
	return regexp.MustCompile(pattern.String())
}
