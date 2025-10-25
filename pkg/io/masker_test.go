package io

import (
	"regexp"
	"testing"
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
	expected := "The secret is ***MASKED*** and more text"
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

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain secret",
			input: "Token: mySecretToken",
			want:  "Token: ***MASKED***",
		},
		{
			name:  "base64 encoded",
			input: "Token: bXlTZWNyZXRUb2tlbg==",
			want:  "Token: ***MASKED***",
		},
		{
			name:  "URL encoded",
			input: "Token: mySecretToken",
			want:  "Token: ***MASKED***",
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
	expected := "Authorization: ***MASKED***"
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

func TestMasker_RegisterRegex(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	m := newMasker(cfg)

	// Test nil regex
	m.RegisterRegex(nil)
	if m.Count() != 0 {
		t.Errorf("expected 0 masks after registering nil regex, got %d", m.Count())
	}

	// Test valid regex
	re := regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`)
	m.RegisterRegex(re)

	input := "GitHub token: ghp_123456789012345678901234567890123456"
	expected := "GitHub token: ***MASKED***"
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
		name     string
		setup    func(Masker)
		input    string
		want     string
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
			want:  "The password is ***MASKED***",
		},
		{
			name: "multiple literals",
			setup: func(m Masker) {
				m.RegisterValue("secret1")
				m.RegisterValue("secret2")
			},
			input: "secret1 and secret2",
			want:  "***MASKED*** and ***MASKED***",
		},
		{
			name: "pattern match",
			setup: func(m Masker) {
				_ = m.RegisterPattern(`password=\w+`)
			},
			input: "login with password=abc123",
			want:  "login with ***MASKED***",
		},
		{
			name: "combined literal and pattern",
			setup: func(m Masker) {
				m.RegisterValue("token123")
				_ = m.RegisterPattern(`api_key=\w+`)
			},
			input: "token123 and api_key=xyz789",
			want:  "***MASKED*** and ***MASKED***",
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
