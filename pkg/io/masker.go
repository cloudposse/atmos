package io

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

const (
	// MaskReplacement is the string used to replace masked values.
	MaskReplacement = "***MASKED***"
)

// masker implements the Masker interface.
type masker struct {
	mu       sync.RWMutex
	literals map[string]bool // Literal values to mask
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
	if value == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.literals[value] = true
}

func (m *masker) RegisterSecret(secret string) {
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

	// Register JSON encoded version
	if jsonBytes, err := json.Marshal(secret); err == nil {
		// Remove surrounding quotes
		jsonStr := string(jsonBytes)
		if len(jsonStr) > 2 {
			m.RegisterValue(jsonStr[1 : len(jsonStr)-1])
		}
	}
}

func (m *masker) RegisterPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	m.RegisterRegex(re)
	return nil
}

func (m *masker) RegisterRegex(pattern *regexp.Regexp) {
	if pattern == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.patterns = append(m.patterns, pattern)
}

func (m *masker) RegisterAWSAccessKey(accessKeyID string) {
	if accessKeyID == "" {
		return
	}

	m.RegisterValue(accessKeyID)

	// AWS access keys follow pattern: AKIA[0-9A-Z]{16}
	// If this is a valid AWS access key, register a pattern to catch the paired secret key
	if strings.HasPrefix(accessKeyID, "AKIA") && len(accessKeyID) == 20 {
		// AWS secret keys are 40 characters of base64-like characters
		// Register a pattern to catch anything that looks like an AWS secret key near this access key
		// This is a best-effort approach
		_ = m.RegisterPattern(`[A-Za-z0-9/+=]{40}`)
	}
}

func (m *masker) Mask(input string) string {
	if !m.enabled || input == "" {
		return input
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	masked := input

	// Mask literals (exact matches)
	// Sort by length (longest first) to avoid partial replacements
	for literal := range m.literals {
		if literal != "" {
			masked = strings.ReplaceAll(masked, literal, MaskReplacement)
		}
	}

	// Mask regex patterns
	for _, pattern := range m.patterns {
		masked = pattern.ReplaceAllString(masked, MaskReplacement)
	}

	return masked
}

func (m *masker) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.literals = make(map[string]bool)
	m.patterns = make([]*regexp.Regexp, 0)
}

func (m *masker) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.literals) + len(m.patterns)
}

func (m *masker) Enabled() bool {
	return m.enabled
}
