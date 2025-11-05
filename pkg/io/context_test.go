package io

import (
	stdIo "io"
	"os"
	"regexp"
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
