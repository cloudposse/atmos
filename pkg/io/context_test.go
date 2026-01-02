package io

import (
	stdIo "io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewContext(t *testing.T) {
	// Reset viper state
	viper.Reset()

	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}

	if ctx == nil {
		t.Fatal("NewContext() returned nil")
	}

	if ctx.Streams() == nil {
		t.Error("Streams() returned nil")
	}

	if ctx.Config() == nil {
		t.Error("Config() returned nil")
	}

	if ctx.Masker() == nil {
		t.Error("Masker() returned nil")
	}
}

func TestBuildConfig(t *testing.T) {
	// Reset viper state before test
	viper.Reset()

	// Test with minimal viper configuration
	viper.Set("redirect-stderr", "")
	viper.Set("mask", true)

	cfg := buildConfig()

	if cfg == nil {
		t.Fatal("buildConfig() returned nil")
	}

	// Test with atmos.yaml config
	viper.Set("settings", schema.Settings{})

	cfg = buildConfig()

	if cfg == nil {
		t.Fatal("buildConfig() with atmos.yaml returned nil")
	}
}

func TestWithOptions(t *testing.T) {
	// Create test streams
	testInput := stdIo.NopCloser(os.Stdin)
	testOutput := os.Stdout
	testError := os.Stderr

	testStreams := &streams{
		input:  testInput,
		output: testOutput,
		error:  testError,
	}

	testMasker := &masker{
		enabled:  true,
		literals: make(map[string]bool),
		patterns: make([]*regexp.Regexp, 0),
	}

	tests := []struct {
		name string
		opts []ContextOption
	}{
		{
			name: "WithStreams option",
			opts: []ContextOption{WithStreams(testStreams)},
		},
		{
			name: "WithMasker option",
			opts: []ContextOption{WithMasker(testMasker)},
		},
		{
			name: "Multiple options",
			opts: []ContextOption{
				WithStreams(testStreams),
				WithMasker(testMasker),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewContext(tt.opts...)
			if err != nil {
				t.Fatalf("NewContext() with options error = %v", err)
			}

			if ctx == nil {
				t.Fatal("NewContext() with options returned nil")
			}
		})
	}
}

func TestStreamString(t *testing.T) {
	tests := []struct {
		name   string
		stream Stream
		want   string
	}{
		{
			name:   "DataStream returns 'data'",
			stream: DataStream,
			want:   "data",
		},
		{
			name:   "UIStream returns 'ui'",
			stream: UIStream,
			want:   "ui",
		},
		{
			name:   "Unknown stream returns 'unknown'",
			stream: Stream(999),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stream.String()
			if got != tt.want {
				t.Errorf("Stream.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaskConfiguration(t *testing.T) {
	tests := []struct {
		name                  string
		maskFlag              *bool  // nil means not set
		maskEnv               string // empty means not set
		configMaskEnabled     *bool  // nil means not set in config
		expectedDisabled      bool   // expected value of DisableMasking (true = masking disabled)
		expectedMaskerEnabled bool   // expected masker.Enabled() result
	}{
		{
			name:                  "Default: masking enabled when nothing set",
			maskFlag:              nil,
			maskEnv:               "",
			configMaskEnabled:     nil,
			expectedDisabled:      false,
			expectedMaskerEnabled: true,
		},
		{
			name:                  "--mask=true enables masking",
			maskFlag:              boolPtr(true),
			maskEnv:               "",
			configMaskEnabled:     nil,
			expectedDisabled:      false,
			expectedMaskerEnabled: true,
		},
		{
			name:                  "--mask=false disables masking",
			maskFlag:              boolPtr(false),
			maskEnv:               "",
			configMaskEnabled:     nil,
			expectedDisabled:      true,
			expectedMaskerEnabled: false,
		},
		{
			name:                  "ATMOS_MASK=true enables masking",
			maskFlag:              nil,
			maskEnv:               "true",
			configMaskEnabled:     nil,
			expectedDisabled:      false,
			expectedMaskerEnabled: true,
		},
		{
			name:                  "ATMOS_MASK=false disables masking",
			maskFlag:              nil,
			maskEnv:               "false",
			configMaskEnabled:     nil,
			expectedDisabled:      true,
			expectedMaskerEnabled: false,
		},
		{
			name:                  "--mask flag overrides env var",
			maskFlag:              boolPtr(true),
			maskEnv:               "false",
			configMaskEnabled:     nil,
			expectedDisabled:      false,
			expectedMaskerEnabled: true,
		},
		{
			name:                  "settings.terminal.mask.enabled=true enables masking",
			maskFlag:              nil,
			maskEnv:               "",
			configMaskEnabled:     boolPtr(true),
			expectedDisabled:      false,
			expectedMaskerEnabled: true,
		},
		{
			name:                  "settings.terminal.mask.enabled=false disables masking",
			maskFlag:              nil,
			maskEnv:               "",
			configMaskEnabled:     boolPtr(false),
			expectedDisabled:      true,
			expectedMaskerEnabled: false,
		},
		{
			name:                  "--mask flag overrides config",
			maskFlag:              boolPtr(false),
			maskEnv:               "",
			configMaskEnabled:     boolPtr(true),
			expectedDisabled:      true,
			expectedMaskerEnabled: false,
		},
		{
			name:                  "env var overrides config",
			maskFlag:              nil,
			maskEnv:               "false",
			configMaskEnabled:     boolPtr(true),
			expectedDisabled:      true,
			expectedMaskerEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper state
			viper.Reset()

			// Set environment variable if specified
			if tt.maskEnv != "" {
				t.Setenv("ATMOS_MASK", tt.maskEnv)
				if err := viper.BindEnv("mask", "ATMOS_MASK"); err != nil {
					t.Fatalf("Failed to bind ATMOS_MASK: %v", err)
				}
			}

			// Set flag if specified
			if tt.maskFlag != nil {
				viper.Set("mask", *tt.maskFlag)
			} else if tt.maskEnv == "" && tt.configMaskEnabled == nil {
				// Only set default when neither flag, env, nor config is specified
				// This ensures config precedence works correctly
				viper.SetDefault("mask", true)
			}

			// Set config if specified
			if tt.configMaskEnabled != nil {
				viper.Set("settings", schema.AtmosSettings{
					Terminal: schema.Terminal{
						Mask: schema.MaskSettings{
							Enabled: *tt.configMaskEnabled,
						},
					},
				})
				viper.Set("settings.terminal.mask.enabled", *tt.configMaskEnabled)
			}

			// Build config
			cfg := buildConfig()

			// Check DisableMasking field
			if cfg.DisableMasking != tt.expectedDisabled {
				t.Errorf("DisableMasking = %v, want %v", cfg.DisableMasking, tt.expectedDisabled)
			}

			// Create masker and check if it respects the configuration
			m := newMasker(cfg)
			if m.Enabled() != tt.expectedMaskerEnabled {
				t.Errorf("Masker.Enabled() = %v, want %v", m.Enabled(), tt.expectedMaskerEnabled)
			}
		})
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// TestNewContextRegistersCommonSecrets verifies that common secret patterns
// are automatically registered when NewContext() is called.
// This test would have caught the bug where registerCommonSecrets() was never invoked.
func TestNewContextRegistersCommonSecrets(t *testing.T) {
	// Reset viper state.
	viper.Reset()
	viper.SetDefault("mask", true)

	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}

	masker := ctx.Masker()
	if masker == nil {
		t.Fatal("Masker() returned nil")
	}

	// Verify masker is enabled.
	if !masker.Enabled() {
		t.Error("Masker should be enabled by default")
	}

	// Test cases: known secret values that should be masked.
	tests := []struct {
		name   string
		input  string
		masked bool
	}{
		{
			name:   "AWS Secret Access Key (via env var - not pattern matched)",
			input:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			masked: false, // AWS secret keys are only masked when set via AWS_SECRET_ACCESS_KEY env var, not by pattern (too generic).
		},
		{
			name:   "AWS Access Key ID",
			input:  "AKIAIOSFODNN7EXAMPLE",
			masked: true,
		},
		{
			name:   "GitHub Personal Access Token (classic)",
			input:  "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			masked: true,
		},
		{
			name:   "GitHub OAuth Token",
			input:  "gho_abcdefghijklmnopqrstuvwxyz1234567890",
			masked: true,
		},
		{
			name:   "GitHub Fine-grained PAT",
			input:  "github_pat_11AAAAAAAAAAAAAAAAAAAA_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			masked: true, // Pattern: github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59} (22 + 59 chars).
		},
		{
			name:   "GitLab Personal Access Token",
			input:  "glpat-abcdefghij1234567890",
			masked: true,
		},
		{
			name:   "OpenAI API Key",
			input:  "sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLM",
			masked: true,
		},
		{
			name:   "Bearer Token",
			input:  "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0=",
			masked: true,
		},
		{
			name:   "Regular text should not be masked",
			input:  "This is just regular text without secrets",
			masked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := masker.Mask(tt.input)

			if tt.masked {
				// Output should be masked (contain MaskReplacement).
				if output == tt.input {
					t.Errorf("Expected input to be masked, but got original value.\nInput:  %s\nOutput: %s", tt.input, output)
				}
				// Check that output contains the mask replacement string.
				if !strings.Contains(output, MaskReplacement) {
					t.Errorf("Expected output to contain '%s', but got: %s", MaskReplacement, output)
				}
			} else if output != tt.input {
				// Output should NOT be masked (unchanged).
				t.Errorf("Expected input to remain unchanged, but it was masked.\nInput:  %s\nOutput: %s", tt.input, output)
			}
		})
	}
}

// TestNewContextWithMaskingDisabled verifies that when masking is disabled,
// secrets are not masked even though patterns are registered.
func TestNewContextWithMaskingDisabled(t *testing.T) {
	// Reset viper state.
	viper.Reset()
	viper.Set("mask", false)

	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}

	masker := ctx.Masker()
	if masker == nil {
		t.Fatal("Masker() returned nil")
	}

	// Verify masker is disabled.
	if masker.Enabled() {
		t.Error("Masker should be disabled when mask=false")
	}

	// Test that secrets are NOT masked when masking is disabled.
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	output := masker.Mask(secretKey)

	if output != secretKey {
		t.Errorf("When masking is disabled, output should be unchanged.\nInput:  %s\nOutput: %s", secretKey, output)
	}
}
