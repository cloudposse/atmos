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
	MaskReplacement = "***MASKED***"

	// AWS access key ID length (AKIA prefix + 16 characters).
	awsAccessKeyIDLength = 20
)

// masker implements the Masker interface.
type masker struct {
	mu       sync.RWMutex
	literals map[string]bool  // Literal values to mask
	patterns []*regexp.Regexp // Regex patterns to mask
	enabled  bool
}

// newMasker creates a new Masker.
func newMasker(config *Config) Masker {
	m := &masker{
		literals: make(map[string]bool),
		patterns: make([]*regexp.Regexp, 0),
		enabled:  !config.DisableMasking,
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

	m.literals[value] = true
}

func (m *masker) RegisterSecret(secret string) {
	defer perf.Track(nil, "io.masker.RegisterSecret")()

	if secret == "" {
		return
	}

	m.RegisterValue(secret)

	// Register base64 encoded versions
	m.RegisterValue(base64.StdEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.URLEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.RawStdEncoding.EncodeToString([]byte(secret)))
	m.RegisterValue(base64.RawURLEncoding.EncodeToString([]byte(secret)))

	// Register URL encoded version
	m.RegisterValue(url.QueryEscape(secret))

	// Register JSON encoded version (both quoted and unquoted)
	if jsonBytes, err := json.Marshal(secret); err == nil {
		jsonStr := string(jsonBytes)
		// Register the full quoted JSON string
		m.RegisterValue(jsonStr)
		// Also register the unquoted inner text
		if len(jsonStr) > 2 {
			m.RegisterValue(jsonStr[1 : len(jsonStr)-1])
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

	if !m.enabled || input == "" {
		return input
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	masked := input

	// Mask literals (exact matches)
	// Sort by length (longest first) to avoid partial replacements
	// Collect literals into slice
	literals := make([]string, 0, len(m.literals))
	for literal := range m.literals {
		if literal != "" {
			literals = append(literals, literal)
		}
	}

	// Sort by length descending (longest first)
	// This prevents shorter literals from being replaced before longer ones
	for i := 0; i < len(literals); i++ {
		for j := i + 1; j < len(literals); j++ {
			if len(literals[j]) > len(literals[i]) {
				literals[i], literals[j] = literals[j], literals[i]
			}
		}
	}

	// Replace literals in order (longest first)
	for _, literal := range literals {
		masked = strings.ReplaceAll(masked, literal, MaskReplacement)
	}

	// Mask regex patterns
	for _, pattern := range m.patterns {
		masked = pattern.ReplaceAllString(masked, MaskReplacement)
	}

	return masked
}

func (m *masker) Clear() {
	defer perf.Track(nil, "io.masker.Clear")()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.literals = make(map[string]bool)
	m.patterns = make([]*regexp.Regexp, 0)
}

func (m *masker) Count() int {
	defer perf.Track(nil, "io.masker.Count")()

	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.literals) + len(m.patterns)
}

func (m *masker) Enabled() bool {
	defer perf.Track(nil, "io.masker.Enabled")()

	return m.enabled
}
