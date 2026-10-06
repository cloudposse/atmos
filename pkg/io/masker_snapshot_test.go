package io

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyMask is the pre-snapshot Mask implementation (per-call bubble sort and regex
// compilation). It is the reference the cached snapshot must stay byte-identical to.
func legacyMask(literalSet map[string]bool, patterns []*regexp.Regexp, replacement, input string) string {
	masked := input
	literals := make([]string, 0, len(literalSet))
	for literal := range literalSet {
		if literal != "" {
			literals = append(literals, literal)
		}
	}
	for i := 0; i < len(literals); i++ {
		for j := i + 1; j < len(literals); j++ {
			if len(literals[j]) > len(literals[i]) {
				literals[i], literals[j] = literals[j], literals[i]
			}
		}
	}
	quoted := strings.ReplaceAll(replacement, "$", "$$")
	for _, literal := range literals {
		masked = strings.ReplaceAll(masked, literal, replacement)
		if re := indentedMultilineRegexp(literal); re != nil {
			masked = re.ReplaceAllString(masked, quoted)
		}
		if re := foldedLiteralRegexp(literal); re != nil {
			masked = re.ReplaceAllString(masked, quoted)
		}
	}
	for _, pattern := range patterns {
		masked = pattern.ReplaceAllString(masked, quoted)
	}
	return masked
}

const testPEM = "-----BEGIN TEST CERTIFICATE-----\nATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA\nATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB\n-----END TEST CERTIFICATE-----\n"

func TestMaskSnapshot_IdenticalToLegacyMask(t *testing.T) {
	type registration struct {
		secrets  []string
		values   []string
		patterns []string
	}
	registrations := map[string]registration{
		"none":       {},
		"single":     {values: []string{"secret123"}},
		"encodings":  {secrets: []string{"mySecretToken", "p@ss w0rd/+="}},
		"overlap":    {values: []string{"abc", "abcdef", "bcd", "secret", "secretvalue"}},
		"dollar":     {values: []string{"tok"}},
		"pem":        {secrets: []string{testPEM}},
		"long-fold":  {values: []string{"this is a long scalar value with spaces inside it"}},
		"patterns":   {patterns: []string{`ghp_[A-Za-z0-9]{6}`, `Bearer [A-Za-z0-9]+`}},
		"mixed":      {secrets: []string{testPEM, "plain-token-1"}, values: []string{"x"}, patterns: []string{`AKIA[0-9A-Z]{4}`}},
		"json-chars": {secrets: []string{"line1\nline2\t\"quoted\""}},
	}

	inputs := []string{
		"",
		"nothing to see here",
		"The secret is secret123 and more text",
		"Token: mySecretToken and bXlTZWNyZXRUb2tlbg==",
		"abcdef abc bcd secretvalue secret",
		"price is $5 tok",
		testPEM,
		"before\n" + testPEM + "after\n",
		"yaml:\n  key: |\n    -----BEGIN TEST CERTIFICATE-----\n    ATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA\n    ATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB\n    -----END TEST CERTIFICATE-----\n",
		"truncated: ATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA only",
		"folded: this is a long scalar\n    value with spaces inside it done",
		"ghp_abc123 Bearer abc123XYZ AKIA1B2C",
		"line1\nline2\t\"quoted\" and line1\\nline2\\t\\\"quoted\\\"",
		"x marks the x spot xx",
	}

	for name, reg := range registrations {
		t.Run(name, func(t *testing.T) {
			m := newMasker(&Config{}).(*masker)
			for _, s := range reg.secrets {
				m.RegisterSecret(s)
			}
			for _, v := range reg.values {
				m.RegisterValue(v)
			}
			for _, p := range reg.patterns {
				require.NoError(t, m.RegisterPattern(p))
			}
			for _, replacement := range []string{MaskReplacement, "$1***", "[redacted]"} {
				m.SetReplacement(replacement)
				for _, input := range inputs {
					want := legacyMask(m.literals, m.patterns, replacement, input)
					assert.Equal(t, want, m.Mask(input), "replacement=%q input=%q", replacement, input)
				}
			}
		})
	}
}

func TestMaskSnapshot_InvalidatedByRegistrationAndClear(t *testing.T) {
	m := newMasker(&Config{})
	assert.Equal(t, "value", m.Mask("value"))

	m.RegisterValue("value")
	assert.Equal(t, MaskReplacement, m.Mask("value"), "registration after a cached Mask must be visible")

	require.NoError(t, m.RegisterPattern(`id-[0-9]+`))
	assert.Equal(t, MaskReplacement+" "+MaskReplacement, m.Mask("value id-42"))

	m.Clear()
	assert.Equal(t, "value id-42", m.Mask("value id-42"), "Clear must drop the cached snapshot")
}

func TestMaskSnapshot_DeterministicOrderForEqualLengths(t *testing.T) {
	m := newMasker(&Config{}).(*masker)
	for _, v := range []string{"bbb", "aaa", "ccc", "dddd"} {
		m.RegisterValue(v)
	}
	snap, _, _ := m.view()
	got := make([]string, 0, len(snap.literals))
	for _, e := range snap.literals {
		got = append(got, e.text)
	}
	assert.Equal(t, []string{"dddd", "aaa", "bbb", "ccc"}, got)
}

func TestMasker_HoldbackLen(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("supersecret")
	m.RegisterValue("abc")
	m.RegisterValue("bcdef")

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"ordinary text", "hello world", 0},
		{"tail is one-byte literal prefix", "hello s", 1},
		{"tail is longer literal prefix", "hello supers", 6},
		{"complete literal is not a proper prefix", "hello supersecret", 0},
		{"mid-word occurrence without prefix tail", "xxsupersecretxx", 0},
		{"straddle moves cut before complete shorter literal", "abcd", 4},
		{"prefix of second literal", "zzbcd", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, m.HoldbackLen(tt.input, true))
		})
	}
}

func TestMasker_HoldbackLen_DisabledAndLineBoundary(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterValue("supersecret")
	m.SetEnabled(false)
	assert.Zero(t, m.HoldbackLen("hello supers", true), "disabled masking never holds output back")
	m.SetEnabled(true)

	// Without patterns the line boundary flag has no effect.
	assert.Zero(t, m.HoldbackLen("partial line", true))

	require.NoError(t, m.RegisterPattern(`Bearer [A-Za-z0-9]+`))
	assert.Equal(t, len("partial"), m.HoldbackLen("done\npartial", true), "regex patterns hold the unfinished line")
	assert.Zero(t, m.HoldbackLen("done\r\n", true), "complete lines are released")
	assert.Zero(t, m.HoldbackLen("done\npartial", false), "line hold can be disabled")
	assert.Equal(t, len("supers"), m.HoldbackLen("done\npartial supers", false), "literals stay protected without line hold")
}

func TestMasker_HoldbackLen_PatternHoldIsBounded(t *testing.T) {
	m := newMasker(&Config{})
	require.NoError(t, m.RegisterPattern(`Bearer [A-Za-z0-9]+`))
	longLine := strings.Repeat("x", maxPatternHold*3)
	assert.Equal(t, maxPatternHold, m.HoldbackLen(longLine, true), "memory for newline-free output stays bounded")
}

func TestMasker_HoldbackLen_MultilineAndIndentedRenderings(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterSecret(testPEM)

	// A YAML-indented rendering is still held back while it could become the secret.
	partial := "key: |\n    -----BEGIN TEST CERTIFICATE-----\n    ATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA\n"
	hold := m.HoldbackLen(partial, true)
	require.Positive(t, hold)
	assert.True(t, strings.HasPrefix(partial[len(partial)-hold:], "-----BEGIN"), "hold starts at the first PEM line, got %q", partial[len(partial)-hold:])

	// Once the whole secret is present and followed by ordinary text nothing needs to be held.
	assert.Zero(t, m.HoldbackLen(partial+"    ATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB\n    -----END TEST CERTIFICATE-----\nnext: 1\n", true))
}

// BenchmarkMask measures Mask with many registered literals, the case the snapshot cache targets.
func BenchmarkMask(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		m := newMasker(&Config{}).(*masker)
		for i := 0; i < n; i++ {
			m.RegisterSecret(fmt.Sprintf("secret-value-%04d-with-some-extra-length", i))
		}
		m.RegisterSecret(testPEM)
		m.RegisterValue("a long registered value that contains several spaces inside of it")
		input := strings.Repeat("ordinary terraform output line without secrets in it\n", 20) + "token secret-value-0003-with-some-extra-length\n"

		b.Run(fmt.Sprintf("snapshot/literals=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = m.Mask(input)
			}
		})
		b.Run(fmt.Sprintf("legacy/literals=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = legacyMask(m.literals, m.patterns, m.replacement, input)
			}
		})
	}
}
