package io

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// MaskReplacement is the string used to replace masked values.
	// Uses angle brackets to be safe in both JSON and YAML output.
	// (Asterisks would be interpreted as YAML alias references).
	MaskReplacement = "<MASKED>"

	// AWS access key ID length (AKIA prefix + 16 characters).
	awsAccessKeyIDLength = 20
)

// masker implements the Masker interface.
type masker struct {
	mu          sync.RWMutex
	literals    map[string]bool  // Literal values to mask.
	patterns    []*regexp.Regexp // Regex patterns to mask.
	enabled     bool
	replacement string // Custom replacement string (default: MaskReplacement).

	// snap caches the sorted literals and compiled regexes derived from literals and patterns.
	// It is rebuilt lazily after any registration or Clear (nil means stale) so that Mask does
	// not re-sort literals or recompile regexes on every call.
	snap *maskSnapshot
}

// newMasker creates a new Masker.
func newMasker(config *Config) Masker {
	// Determine replacement string from config or use default.
	replacement := MaskReplacement
	if config != nil {
		if r := config.AtmosConfig.Settings.Terminal.Mask.Replacement; r != "" {
			replacement = r
		}
	}

	enabled := true
	if config != nil {
		enabled = !config.DisableMasking
	}

	m := &masker{
		literals:    make(map[string]bool),
		patterns:    make([]*regexp.Regexp, 0),
		enabled:     enabled,
		replacement: replacement,
	}

	return m
}

func (m *masker) RegisterValue(value string) {
	defer perf.Track(nil, "io.masker.RegisterValue")()

	if value == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.literals[value] {
		m.literals[value] = true
		m.snap = nil
	}
}

func (m *masker) RegisterSecret(secret string) {
	defer perf.Track(nil, "io.masker.RegisterSecret")()

	if secret == "" {
		return
	}

	m.RegisterValue(secret)
	m.registerMultilineSecretLines(secret)

	// Register base64 encoded versions.
	m.RegisterValue(base64.StdEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.URLEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.RawStdEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.RawURLEncoding.EncodeToString([]byte(secret)))

	// Register URL encoded version.
	m.RegisterValue(url.QueryEscape(secret))

	// Register JSON-escaped version (without quotes to preserve JSON structure).
	// We only register the inner content, not the full quoted string.
	// This ensures "secret" becomes "<MASKED>" (valid JSON), not <MASKED> (invalid).
	if jsonBytes, err := json.Marshal(secret); err == nil {
		jsonStr := string(jsonBytes)
		// Only register the escaped inner text (handles special chars like \n, \t, etc.).
		if len(jsonStr) > 2 {
			escapedInner := jsonStr[1 : len(jsonStr)-1]
			// Only register if different from plain secret (has escaping).
			if escapedInner != secret {
				m.RegisterValue(escapedInner)
			}
		}
	}
}

// registerMultilineSecretLines protects partial renderings where a formatter or diff
// elides part of a multiline secret and the complete registered literal is no longer
// present. Empty lines are ignored because they carry no secret information.
func (m *masker) registerMultilineSecretLines(secret string) {
	normalized := strings.ReplaceAll(secret, "\r\n", "\n")
	if !strings.Contains(normalized, "\n") {
		return
	}
	for _, line := range strings.Split(normalized, "\n") {
		if line != "" {
			m.RegisterValue(line)
		}
	}
}

func (m *masker) RegisterPattern(pattern string) error {
	defer perf.Track(nil, "io.masker.RegisterPattern")()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	m.RegisterRegex(re)
	return nil
}

func (m *masker) RegisterRegex(pattern *regexp.Regexp) {
	defer perf.Track(nil, "io.masker.RegisterRegex")()

	if pattern == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.patterns = append(m.patterns, pattern)
	m.snap = nil
}

func (m *masker) RegisterAWSAccessKey(accessKeyID string) {
	defer perf.Track(nil, "io.masker.RegisterAWSAccessKey")()

	if accessKeyID == "" {
		return
	}

	m.RegisterValue(accessKeyID)

	// If this looks like an AWS access key, also mask the paired secret when labeled.
	if len(accessKeyID) == awsAccessKeyIDLength && (strings.HasPrefix(accessKeyID, "AKIA") || strings.HasPrefix(accessKeyID, "ASIA")) {
		// Match common labeling to reduce false positives.
		_ = m.RegisterPattern(`(?i)\bAWS_SECRET_ACCESS_KEY\b[=:]\s*[A-Za-z0-9/+=]{40}`)
	}
}

func (m *masker) Mask(input string) string {
	defer perf.Track(nil, "io.masker.Mask")()

	if input == "" {
		return input
	}

	snap, enabled, replacement := m.view()
	if !enabled {
		return input
	}

	return snap.mask(input, replacement)
}

// HoldbackLen implements Masker.HoldbackLen.
func (m *masker) HoldbackLen(input string, lineBoundary bool) int {
	defer perf.Track(nil, "io.masker.HoldbackLen")()

	if input == "" {
		return 0
	}

	snap, enabled, _ := m.view()
	if !enabled {
		return 0
	}

	return snap.holdbackLen(input, lineBoundary)
}

// view returns the current immutable snapshot together with the enabled flag and replacement,
// building the snapshot first when a registration invalidated it.
func (m *masker) view() (snap *maskSnapshot, enabled bool, replacement string) {
	m.mu.RLock()
	if m.snap != nil {
		snap, enabled, replacement = m.snap, m.enabled, m.replacement
		m.mu.RUnlock()
		return snap, enabled, replacement
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snap == nil {
		m.snap = buildMaskSnapshot(m.literals, m.patterns)
	}
	return m.snap, m.enabled, m.replacement
}

// ContainsSecret reports whether value contains any registered secret literal as a
// substring. This deliberately ignores the enabled flag (unlike Mask): callers use it
// to prevent secrets from being written to disk (e.g. Terraform varfiles) even when
// display masking is disabled via --mask=false. Regex patterns are not consulted here —
// only literal secret values, which are exactly what must be kept off disk.
func (m *masker) ContainsSecret(value string) bool {
	defer perf.Track(nil, "io.masker.ContainsSecret")()

	if value == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for literal := range m.literals {
		if literal != "" && strings.Contains(value, literal) {
			return true
		}
	}

	return false
}

func (m *masker) Clear() {
	defer perf.Track(nil, "io.masker.Clear")()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.literals = make(map[string]bool)
	m.patterns = make([]*regexp.Regexp, 0)
	m.snap = nil
}

func (m *masker) Count() int {
	defer perf.Track(nil, "io.masker.Count")()

	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.literals) + len(m.patterns)
}

func (m *masker) Enabled() bool {
	defer perf.Track(nil, "io.masker.Enabled")()

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

func (m *masker) SetEnabled(enabled bool) {
	defer perf.Track(nil, "io.masker.SetEnabled")()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

func (m *masker) SetReplacement(replacement string) {
	defer perf.Track(nil, "io.masker.SetReplacement")()

	if replacement == "" {
		replacement = MaskReplacement
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.replacement = replacement
}

func (m *masker) Replacement() string {
	defer perf.Track(nil, "io.masker.Replacement")()

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.replacement
}
