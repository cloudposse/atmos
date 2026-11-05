package io

import (
	"math"
	"regexp"
	"regexp/syntax"
	"unicode"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
)

// maskPatternSpan returns the maximum bytes a cross-line match can consume, or zero
// for line-local patterns. Unbounded cross-line patterns cannot be streamed safely.
func maskPatternSpan(pattern *regexp.Regexp) (int, error) {
	tree, err := syntax.Parse(pattern.String(), syntax.Perl)
	if err != nil {
		return 0, err
	}
	span, crossLine := regexSpan(tree)
	if !crossLine {
		return 0, nil
	}
	if span < 0 {
		return 0, errUtils.ErrUnboundedMaskPattern
	}
	return span, nil
}

// regexSpan analyzes consuming expressions, including expanded Unicode character classes.
// A negative width represents an unbounded or unrepresentable maximum byte length.
func regexSpan(tree *syntax.Regexp) (int, bool) {
	switch tree.Op {
	case syntax.OpLiteral:
		return literalRegexSpan(tree)
	case syntax.OpCharClass:
		return classRegexSpan(tree)
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return utf8.UTFMax, true // Even a dot without (?s) can consume a carriage return.
	case syntax.OpCapture, syntax.OpQuest:
		return regexSpan(tree.Sub[0])
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		return repeatedRegexSpan(tree)
	case syntax.OpConcat, syntax.OpAlternate:
		return combinedRegexSpan(tree)
	default:
		return 0, false // Anchors, assertions, empty matches, and impossible matches consume no bytes.
	}
}

func repeatRegexSpan(span, count int) int {
	if span == 0 || count == 0 {
		return 0
	}
	if span < 0 || count < 0 || span > math.MaxInt/count {
		return -1
	}
	return span * count
}

func combinedRegexSpan(tree *syntax.Regexp) (int, bool) {
	span, crossLine := 0, false
	for _, child := range tree.Sub {
		width, crosses := regexSpan(child)
		crossLine = crossLine || crosses
		switch {
		case span < 0 || width < 0:
			span = -1
		case tree.Op == syntax.OpAlternate:
			span = max(span, width)
		case span > math.MaxInt-width:
			span = -1
		default:
			span += width
		}
	}
	return span, crossLine
}

// regexRuneWidth includes multibyte case-fold equivalents such as the Kelvin sign for K.
func regexRuneWidth(r rune, foldCase bool) int {
	width := utf8.RuneLen(r)
	if foldCase {
		for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
			width = max(width, utf8.RuneLen(folded))
		}
	}
	return width
}

func literalRegexSpan(tree *syntax.Regexp) (int, bool) {
	span, crossLine := 0, false
	for _, r := range tree.Rune {
		span += regexRuneWidth(r, tree.Flags&syntax.FoldCase != 0)
		crossLine = crossLine || r == '\r' || r == '\n'
	}
	return span, crossLine
}

func classRegexSpan(tree *syntax.Regexp) (int, bool) {
	span, crossLine := 0, false
	for i := 0; i < len(tree.Rune); i += 2 {
		lo, hi := tree.Rune[i], tree.Rune[i+1]
		span = max(span, utf8.RuneLen(hi))
		crossLine = crossLine || (lo <= '\r' && hi >= '\r') || (lo <= '\n' && hi >= '\n')
	}
	return span, crossLine
}

func repeatedRegexSpan(tree *syntax.Regexp) (int, bool) {
	span, crossLine := regexSpan(tree.Sub[0])
	count := -1
	if tree.Op == syntax.OpRepeat {
		count = tree.Max
	}
	return repeatRegexSpan(span, count), crossLine && count != 0
}
