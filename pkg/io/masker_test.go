package io

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMasker_RegisterValue(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Test empty value
	m.RegisterValue("")
	if m.Count() != 0 {
		t.Errorf("expected 0 masks after registering empty value, got %d", m.Count())
	}

	// Test single value
	m.RegisterValue("secret123")
	if m.Count() != 1 {
		t.Errorf("expected 1 mask, got %d", m.Count())
	}

	// Test masking
	input := "The secret is secret123 and more text"
	expected := "The secret is <MASKED> and more text"
	got := m.Mask(input)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestMasker_RegisterSecret(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	secret := "mySecretToken"
	m.RegisterSecret(secret)

	// Generate encodings at runtime to avoid embedded secrets
	plainSecret := "mySecretToken"
	base64Secret := base64.StdEncoding.EncodeToString([]byte(plainSecret))
	urlSecret := url.QueryEscape(plainSecret)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain secret",
			input: "Token: " + plainSecret,
			want:  "Token: <MASKED>",
		},
		{
			name:  "base64 encoded",
			input: "Token: " + base64Secret,
			want:  "Token: <MASKED>",
		},
		{
			name:  "URL encoded",
			input: "Token: " + urlSecret,
			want:  "Token: <MASKED>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Mask(tt.input)
			if got != tt.want {
				t.Errorf("Mask() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMasker_RegisterPattern(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Test valid pattern
	err := m.RegisterPattern(`Bearer [A-Za-z0-9]+`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test masking
	input := "Authorization: Bearer abc123xyz"
	expected := "Authorization: <MASKED>"
	got := m.Mask(input)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	// Test invalid pattern
	err = m.RegisterPattern(`[invalid(`)
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestMasker_DollarSignInReplacement(t *testing.T) {
	// Test that $ in custom replacement strings is treated literally, not as backreference.
	cfg := &Config{
		DisableMasking: false,
		AtmosConfig: schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Mask: schema.MaskSettings{
						Replacement: "$REDACTED",
					},
				},
			},
		},
	}
	m := newMasker(cfg)

	// Register a pattern that would capture a group.
	err := m.RegisterPattern(`secret-([a-z]+)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// If $ is not escaped, $1 would be replaced with "abc" (the captured group).
	// We want the literal string "$REDACTED" instead.
	input := "token: secret-abc"
	expected := "token: $REDACTED"
	got := m.Mask(input)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestMasker_RegisterRegex(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Test nil regex
	m.RegisterRegex(nil)
	if m.Count() != 0 {
		t.Errorf("expected 0 masks after registering nil regex, got %d", m.Count())
	}

	// Test valid regex - construct token at runtime to avoid secret detection
	re := regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`)
	m.RegisterRegex(re)

	// Generate 36-character suffix at runtime
	tokenSuffix := strings.Repeat("0", 18) + strings.Repeat("1", 18)
	generatedToken := fmt.Sprintf("ghp_%s", tokenSuffix)

	input := fmt.Sprintf("GitHub token: %s", generatedToken)
	expected := "GitHub token: <MASKED>"
	got := m.Mask(input)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestMasker_RegisterAWSAccessKey(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	accessKey := "AKIAIOSFODNN7EXAMPLE"
	m.RegisterAWSAccessKey(accessKey)

	input := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"
	got := m.Mask(input)
	if got == input {
		t.Error("expected access key to be masked")
	}
}

func TestMasker_Mask(t *testing.T) {
	tests := []struct {
		name  string
		setup func(Masker)
		input string
		want  string
	}{
		{
			name: "empty input",
			setup: func(m Masker) {
				m.RegisterValue("secret")
			},
			input: "",
			want:  "",
		},
		{
			name: "no masks registered",
			setup: func(m Masker) {
				// No masks
			},
			input: "plain text",
			want:  "plain text",
		},
		{
			name: "single literal",
			setup: func(m Masker) {
				m.RegisterValue("password123")
			},
			input: "The password is password123",
			want:  "The password is <MASKED>",
		},
		{
			name: "multiple literals",
			setup: func(m Masker) {
				m.RegisterValue("secret1")
				m.RegisterValue("secret2")
			},
			input: "secret1 and secret2",
			want:  "<MASKED> and <MASKED>",
		},
		{
			name: "pattern match",
			setup: func(m Masker) {
				_ = m.RegisterPattern(`password=\w+`)
			},
			input: "login with password=abc123",
			want:  "login with <MASKED>",
		},
		{
			name: "combined literal and pattern",
			setup: func(m Masker) {
				m.RegisterValue("token123")
				_ = m.RegisterPattern(`api_key=\w+`)
			},
			input: "token123 and api_key=xyz789",
			want:  "<MASKED> and <MASKED>",
		},
		{
			name: "json structure preserved",
			setup: func(m Masker) {
				m.RegisterSecret("mysecret")
			},
			input: `{"key": "mysecret", "other": "value"}`,
			want:  `{"key": "<MASKED>", "other": "value"}`,
		},
		{
			name: "yaml structure preserved",
			setup: func(m Masker) {
				m.RegisterSecret("mysecret")
			},
			input: "key: mysecret\nother: value",
			want:  "key: <MASKED>\nother: value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DisableMasking: false}
			m := newMasker(cfg)
			tt.setup(m)

			got := m.Mask(tt.input)
			if got != tt.want {
				t.Errorf("Mask() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMasker_Clear(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	m.RegisterValue("secret1")
	m.RegisterValue("secret2")
	_ = m.RegisterPattern(`token=\w+`)

	if m.Count() == 0 {
		t.Error("expected masks to be registered")
	}

	m.Clear()

	if m.Count() != 0 {
		t.Errorf("expected 0 masks after Clear(), got %d", m.Count())
	}

	// Test that masking doesn't happen after clear
	input := "secret1 and token=abc"
	got := m.Mask(input)
	if got != input {
		t.Errorf("expected no masking after Clear(), got %q", got)
	}
}

func TestMasker_Count(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	if m.Count() != 0 {
		t.Errorf("expected 0 masks initially, got %d", m.Count())
	}

	m.RegisterValue("secret1")
	if m.Count() != 1 {
		t.Errorf("expected 1 mask, got %d", m.Count())
	}

	m.RegisterValue("secret2")
	if m.Count() != 2 {
		t.Errorf("expected 2 masks, got %d", m.Count())
	}

	_ = m.RegisterPattern(`token=\w+`)
	if m.Count() != 3 {
		t.Errorf("expected 3 masks, got %d", m.Count())
	}
}

func TestMasker_Enabled(t *testing.T) {
	tests := []struct {
		name           string
		disableMasking bool
		want           bool
	}{
		{
			name:           "masking enabled",
			disableMasking: false,
			want:           true,
		},
		{
			name:           "masking disabled",
			disableMasking: true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DisableMasking: tt.disableMasking}
			m := newMasker(cfg)

			if m.Enabled() != tt.want {
				t.Errorf("Enabled() = %v, want %v", m.Enabled(), tt.want)
			}
		})
	}
}

func TestMasker_DisabledMasking(t *testing.T) {
	cfg := &Config{DisableMasking: true}
	m := newMasker(cfg)

	m.RegisterValue("secret123")

	input := "The secret is secret123"
	got := m.Mask(input)

	// When masking is disabled, input should be returned unchanged
	if got != input {
		t.Errorf("expected input to be unchanged when masking disabled, got %q", got)
	}
}

func TestMasker_NilConfig(t *testing.T) {
	// Test that newMasker handles nil config gracefully.
	m := newMasker(nil)

	// Should default to enabled.
	if !m.Enabled() {
		t.Error("expected masking to be enabled by default with nil config")
	}

	// Should use default replacement.
	m.RegisterValue("secret")
	got := m.Mask("secret value")
	if got != "<MASKED> value" {
		t.Errorf("expected default replacement, got %q", got)
	}
}

func TestMasker_CustomReplacement(t *testing.T) {
	cfg := &Config{
		DisableMasking: false,
		AtmosConfig: schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Mask: schema.MaskSettings{
						Replacement: "[REDACTED]",
					},
				},
			},
		},
	}
	m := newMasker(cfg)

	m.RegisterValue("secret123")
	input := "The secret is secret123"
	expected := "The secret is [REDACTED]"
	got := m.Mask(input)

	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestMasker_EmptyReplacement(t *testing.T) {
	// When replacement is empty string, should use default.
	cfg := &Config{
		DisableMasking: false,
		AtmosConfig: schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Mask: schema.MaskSettings{
						Replacement: "", // Empty should fall back to default.
					},
				},
			},
		},
	}
	m := newMasker(cfg)

	m.RegisterValue("secret")
	got := m.Mask("secret value")
	if got != "<MASKED> value" {
		t.Errorf("expected default replacement with empty config, got %q", got)
	}
}

func TestRegisterCustomMaskPatterns(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *Config
		input      string
		wantMasked string
	}{
		{
			name:       "nil config",
			cfg:        nil,
			input:      "test secret123",
			wantMasked: "test secret123", // No masking with nil config.
		},
		{
			name: "custom literals",
			cfg: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Mask: schema.MaskSettings{
								Literals: []string{"secret123", "password456"},
							},
						},
					},
				},
			},
			input:      "secret123 and password456",
			wantMasked: "<MASKED> and <MASKED>",
		},
		{
			name: "custom patterns",
			cfg: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Mask: schema.MaskSettings{
								Patterns: []string{`api-key-[a-z0-9]+`},
							},
						},
					},
				},
			},
			input:      "token: api-key-abc123",
			wantMasked: "token: <MASKED>",
		},
		{
			name: "empty literals and patterns are skipped",
			cfg: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Mask: schema.MaskSettings{
								Literals: []string{"", "secret"},
								Patterns: []string{"", `key-\d+`},
							},
						},
					},
				},
			},
			input:      "secret and key-123",
			wantMasked: "<MASKED> and <MASKED>",
		},
		{
			name: "invalid pattern is skipped with warning",
			cfg: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Mask: schema.MaskSettings{
								Patterns: []string{`[invalid(`, `valid-\d+`},
							},
						},
					},
				},
			},
			input:      "valid-123",
			wantMasked: "<MASKED>", // Invalid pattern skipped, valid one works.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DisableMasking: false}
			m := newMasker(cfg)

			// Register custom patterns from config.
			registerCustomMaskPatterns(m, tt.cfg)

			got := m.Mask(tt.input)
			if got != tt.wantMasked {
				t.Errorf("Mask() = %q, want %q", got, tt.wantMasked)
			}
		})
	}
}

func TestMasker_RegisterAWSAccessKey_Empty(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Empty access key should be a no-op.
	m.RegisterAWSAccessKey("")
	if m.Count() != 0 {
		t.Errorf("expected 0 masks for empty access key, got %d", m.Count())
	}
}

func TestMasker_RegisterAWSAccessKey_NonAWS(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Non-AWS format access key (wrong length or prefix).
	m.RegisterAWSAccessKey("NOTAWS123")
	// Should still register the value itself.
	if m.Count() != 1 {
		t.Errorf("expected 1 mask, got %d", m.Count())
	}
}

func TestMasker_RegisterSecret_Empty(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	m.RegisterSecret("")
	if m.Count() != 0 {
		t.Errorf("expected 0 masks for empty secret, got %d", m.Count())
	}
}

func TestMasker_RegisterPattern_Empty(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Empty pattern string should work but add nothing useful.
	err := m.RegisterPattern("")
	if err != nil {
		t.Errorf("unexpected error for empty pattern: %v", err)
	}
}

func TestMasker_RegisterSecret_WithEscaping(t *testing.T) {
	// Test that RegisterSecret handles secrets that need JSON escaping.
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Register a secret with special characters that need JSON escaping.
	secretWithNewline := "secret\nwith\nnewlines"
	m.RegisterSecret(secretWithNewline)

	// The escaped version should also be masked.
	input := `{"value": "secret\nwith\nnewlines"}`
	expected := `{"value": "<MASKED>"}`
	got := m.Mask(input)

	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestMasker_LongestMatchFirst(t *testing.T) {
	// Test that longer literals are masked before shorter ones.
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Register a short and long literal where short is prefix of long.
	m.RegisterValue("secret")
	m.RegisterValue("secret123")

	input := "The value is secret123"
	expected := "The value is <MASKED>"
	got := m.Mask(input)

	// Should mask the longer match, not leave "123" behind.
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestGlobalRegisterSecret(t *testing.T) {
	// Reset global state for testing.
	Reset()

	// Call RegisterSecret before initialization.
	RegisterSecret("test-secret-global")

	// Initialize should have been called.
	ctx := GetContext()
	if ctx == nil {
		t.Fatal("expected context to be initialized")
	}

	// Secret should be registered.
	if ctx.Masker().Count() == 0 {
		t.Error("expected at least one mask registered")
	}

	// Clean up.
	Reset()
}

func TestGlobalRegisterValue(t *testing.T) {
	// Reset global state for testing.
	Reset()

	// Call RegisterValue before initialization.
	RegisterValue("test-value-global")

	// Initialize should have been called.
	ctx := GetContext()
	if ctx == nil {
		t.Fatal("expected context to be initialized")
	}

	// Value should be registered.
	if ctx.Masker().Count() == 0 {
		t.Error("expected at least one mask registered")
	}

	// Clean up.
	Reset()
}

func TestGlobalRegisterPattern(t *testing.T) {
	// Reset global state for testing.
	Reset()

	// Call RegisterPattern before initialization.
	err := RegisterPattern(`test-pattern-\d+`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Initialize should have been called.
	ctx := GetContext()
	if ctx == nil {
		t.Fatal("expected context to be initialized")
	}

	// Clean up.
	Reset()
}

func TestGlobalRegisterPattern_Empty(t *testing.T) {
	// Empty pattern should be a no-op.
	err := RegisterPattern("")
	if err != nil {
		t.Errorf("unexpected error for empty pattern: %v", err)
	}
}

func TestGlobalRegisterSecret_Empty(t *testing.T) {
	// Empty secret should be a no-op.
	RegisterSecret("")
	// No panic or error expected.
}

func TestGlobalRegisterValue_Empty(t *testing.T) {
	// Empty value should be a no-op.
	RegisterValue("")
	// No panic or error expected.
}
