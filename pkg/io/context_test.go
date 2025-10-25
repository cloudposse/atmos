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

	if ctx.Terminal() == nil {
		t.Error("Terminal() returned nil")
	}

	if ctx.Config() == nil {
		t.Error("Config() returned nil")
	}

	if ctx.Masker() == nil {
		t.Error("Masker() returned nil")
	}
}

func TestConfig_ShouldUseColor(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		isTTY  bool
		want   bool
	}{
		{
			name: "NO_COLOR disables color",
			config: &Config{
				EnvNoColor: true,
			},
			isTTY: true,
			want:  false,
		},
		{
			name: "CLICOLOR=0 disables color",
			config: &Config{
				EnvCLIColor: "0",
			},
			isTTY: true,
			want:  false,
		},
		{
			name: "CLICOLOR_FORCE overrides CLICOLOR=0",
			config: &Config{
				EnvCLIColor:      "0",
				EnvCLIColorForce: true,
			},
			isTTY: false,
			want:  true,
		},
		{
			name: "CLICOLOR_FORCE enables color for non-TTY",
			config: &Config{
				EnvCLIColorForce: true,
			},
			isTTY: false,
			want:  true,
		},
		{
			name: "--no-color flag disables color",
			config: &Config{
				NoColor: true,
			},
			isTTY: true,
			want:  false,
		},
		{
			name: "--color flag enables color",
			config: &Config{
				Color: true,
			},
			isTTY: true,
			want:  true,
		},
		{
			name: "default with TTY",
			config: &Config{
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  true,
		},
		{
			name: "default without TTY",
			config: &Config{
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: false,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldUseColor(tt.isTTY)
			if got != tt.want {
				t.Errorf("ShouldUseColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_DetectColorProfile(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		isTTY  bool
		want   ColorProfile
	}{
		{
			name: "NO_COLOR returns ColorNone",
			config: &Config{
				EnvNoColor: true,
			},
			isTTY: true,
			want:  ColorNone,
		},
		{
			name: "COLORTERM=truecolor returns ColorTrue",
			config: &Config{
				EnvColorTerm: "truecolor",
				AtmosConfig:  newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  ColorTrue,
		},
		{
			name: "COLORTERM=24bit returns ColorTrue",
			config: &Config{
				EnvColorTerm: "24bit",
				AtmosConfig:  newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  ColorTrue,
		},
		{
			name: "TERM=xterm-256color returns Color256",
			config: &Config{
				EnvTerm:     "xterm-256color",
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  Color256,
		},
		{
			name: "TERM=screen-256 returns Color256",
			config: &Config{
				EnvTerm:     "screen-256",
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  Color256,
		},
		{
			name: "TERM=xterm returns Color16",
			config: &Config{
				EnvTerm:     "xterm",
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  Color16,
		},
		{
			name: "TERM=screen returns Color16",
			config: &Config{
				EnvTerm:     "screen",
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  Color16,
		},
		{
			name: "non-TTY returns ColorNone",
			config: &Config{
				EnvTerm:     "xterm-256color",
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: false,
			want:  ColorNone,
		},
		{
			name: "default TTY returns Color16",
			config: &Config{
				AtmosConfig: newDefaultAtmosConfig(),
			},
			isTTY: true,
			want:  Color16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.DetectColorProfile(tt.isTTY)
			if got != tt.want {
				t.Errorf("DetectColorProfile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorProfile_String(t *testing.T) {
	tests := []struct {
		profile ColorProfile
		want    string
	}{
		{ColorNone, "None"},
		{Color16, "16"},
		{Color256, "256"},
		{ColorTrue, "TrueColor"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.profile.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamType_String(t *testing.T) {
	tests := []struct {
		stream StreamType
		want   string
	}{
		{StreamInput, "stdin"},
		{StreamOutput, "stdout"},
		{StreamError, "stderr"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.stream.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	// Save original env
	origNoColor := os.Getenv("NO_COLOR")
	origCLIColor := os.Getenv("CLICOLOR")
	origCLIColorForce := os.Getenv("CLICOLOR_FORCE")
	origTerm := os.Getenv("TERM")
	origColorTerm := os.Getenv("COLORTERM")

	// Restore env after test
	defer func() {
		os.Setenv("NO_COLOR", origNoColor)
		os.Setenv("CLICOLOR", origCLIColor)
		os.Setenv("CLICOLOR_FORCE", origCLIColorForce)
		os.Setenv("TERM", origTerm)
		os.Setenv("COLORTERM", origColorTerm)
	}()

	// Reset viper
	viper.Reset()

	// Set test env vars
	os.Setenv("NO_COLOR", "1")
	os.Setenv("CLICOLOR", "0")
	os.Setenv("TERM", "xterm-256color")

	// Set test flags
	viper.Set("no-color", true)
	viper.Set("color", false)

	cfg, err := buildConfig()
	if err != nil {
		t.Fatalf("buildConfig() error = %v", err)
	}

	if !cfg.EnvNoColor {
		t.Error("expected EnvNoColor to be true")
	}

	if cfg.EnvCLIColor != "0" {
		t.Errorf("expected EnvCLIColor to be '0', got %q", cfg.EnvCLIColor)
	}

	if cfg.EnvTerm != "xterm-256color" {
		t.Errorf("expected EnvTerm to be 'xterm-256color', got %q", cfg.EnvTerm)
	}

	if !cfg.NoColor {
		t.Error("expected NoColor flag to be true")
	}
}

func TestWithOptions(t *testing.T) {
	// Test WithStreams option
	t.Run("WithStreams", func(t *testing.T) {
		mockStreams := &mockStreams{}
		ctx, err := NewContext(WithStreams(mockStreams))
		if err != nil {
			t.Fatalf("NewContext() error = %v", err)
		}
		// Verify custom streams were set by checking it's not nil
		if ctx.Streams() == nil {
			t.Error("WithStreams() did not set custom streams")
		}
	})

	// Test WithTerminal option
	t.Run("WithTerminal", func(t *testing.T) {
		mockTerm := &mockTerminal{}
		ctx, err := NewContext(WithTerminal(mockTerm))
		if err != nil {
			t.Fatalf("NewContext() error = %v", err)
		}
		// Verify terminal was set - check that it returns our mock's color profile
		if ctx.Terminal().ColorProfile() != ColorNone {
			t.Error("WithTerminal() did not set custom terminal (wrong color profile)")
		}
	})

	// Test WithMasker option
	t.Run("WithMasker", func(t *testing.T) {
		mockMask := &mockMasker{}
		ctx, err := NewContext(WithMasker(mockMask))
		if err != nil {
			t.Fatalf("NewContext() error = %v", err)
		}
		// Verify masker was set - check it's not nil
		if ctx.Masker() == nil {
			t.Error("WithMasker() did not set custom masker")
		}
	})

	// Test combining multiple options
	t.Run("Multiple options", func(t *testing.T) {
		mockStreams := &mockStreams{}
		mockTerm := &mockTerminal{}
		mockMask := &mockMasker{}

		ctx, err := NewContext(
			WithStreams(mockStreams),
			WithTerminal(mockTerm),
			WithMasker(mockMask),
		)
		if err != nil {
			t.Fatalf("NewContext() error = %v", err)
		}

		if ctx.Streams() == nil {
			t.Error("Multiple options did not preserve streams")
		}
		if ctx.Terminal() == nil {
			t.Error("Multiple options did not preserve terminal")
		}
		if ctx.Masker() == nil {
			t.Error("Multiple options did not preserve masker")
		}
	})
}

// Mock types for option testing.
type mockStreams struct{}

func (m *mockStreams) Input() stdIo.Reader  { return os.Stdin }
func (m *mockStreams) Output() stdIo.Writer { return os.Stdout }
func (m *mockStreams) Error() stdIo.Writer  { return os.Stderr }
func (m *mockStreams) RawOutput() stdIo.Writer { return os.Stdout }
func (m *mockStreams) RawError() stdIo.Writer  { return os.Stderr }

type mockTerminal struct{}

func (m *mockTerminal) ColorProfile() ColorProfile  { return ColorNone }
func (m *mockTerminal) IsTTY(stream interface{}) bool         { return false }
func (m *mockTerminal) Width(stream interface{}) int          { return 80 }
func (m *mockTerminal) Height(stream interface{}) int         { return 24 }
func (m *mockTerminal) SetTitle(title string)                {}
func (m *mockTerminal) RestoreTitle()                        {}
func (m *mockTerminal) Alert()                               {}

type mockMasker struct{}

func (m *mockMasker) RegisterValue(value string)               {}
func (m *mockMasker) RegisterSecret(secret string)             {}
func (m *mockMasker) RegisterPattern(pattern string) error     { return nil }
func (m *mockMasker) RegisterRegex(pattern *regexp.Regexp)     {}
func (m *mockMasker) RegisterAWSAccessKey(accessKeyID string)  {}
func (m *mockMasker) Mask(input string) string                 { return input }
func (m *mockMasker) Clear()                                   {}
func (m *mockMasker) Count() int                               { return 0 }
func (m *mockMasker) Enabled() bool                            { return true }

func TestColorProfile_String_Unknown(t *testing.T) {
	// Test invalid color profile
	profile := ColorProfile(999)
	got := profile.String()
	if got != "Unknown" {
		t.Errorf("String() for invalid profile = %v, want Unknown", got)
	}
}

func TestStreamType_String_Unknown(t *testing.T) {
	// Test invalid stream type
	stream := StreamType(999)
	got := stream.String()
	if got != "unknown" {
		t.Errorf("String() for invalid stream = %v, want unknown", got)
	}
}

// Helper to create default AtmosConfiguration for tests.
func newDefaultAtmosConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Color:   true,
				NoColor: false,
			},
		},
	}
}
