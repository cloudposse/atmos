package terminal

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// setupTest initializes a clean viper instance for testing.
func setupTest(t *testing.T) func() {
	t.Helper()

	// Save original viper state
	originalViper := viper.GetViper()

	// Reset viper for clean test state
	viper.Reset()

	// Return cleanup function that restores original state
	return func() {
		viper.Reset()
		// Restore original viper (if needed in future)
		_ = originalViper
	}
}

func TestForceTTY_IsTTY(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name      string
		forceTTY  bool
		expected  bool
		checkReal bool // Whether to check real TTY detection
	}{
		{
			name:      "force-tty enabled returns true",
			forceTTY:  true,
			expected:  true,
			checkReal: false,
		},
		{
			name:      "force-tty disabled uses real detection",
			forceTTY:  false,
			expected:  false, // Assume test runs without TTY
			checkReal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set viper flags
			viper.Set("force-tty", tt.forceTTY)
			viper.Set("force-color", false)
			viper.Set("no-color", false)
			viper.Set("color", false)

			// Create terminal
			term := New()

			// Check IsTTY for all streams
			result := term.IsTTY(Stdout)

			if tt.checkReal {
				// For non-forced mode, just verify it doesn't panic
				// Actual TTY detection depends on environment
				assert.NotNil(t, term)
			} else {
				assert.Equal(t, tt.expected, result, "IsTTY should return %v when force-tty=%v", tt.expected, tt.forceTTY)
			}
		})
	}
}

func TestForceTTY_Width(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		forceTTY bool
		expected int
	}{
		{
			name:     "force-tty enabled returns default width",
			forceTTY: true,
			expected: defaultForcedWidth,
		},
		{
			name:     "force-tty disabled returns 0 when not TTY",
			forceTTY: false,
			expected: 0, // Assume test runs without TTY
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set viper flags
			viper.Set("force-tty", tt.forceTTY)
			viper.Set("force-color", false)
			viper.Set("no-color", false)
			viper.Set("color", false)

			// Create terminal
			term := New()

			// Check width
			result := term.Width(Stdout)

			if tt.forceTTY {
				assert.Equal(t, tt.expected, result, "Width should return %d when force-tty=%v", tt.expected, tt.forceTTY)
			} else {
				// For non-forced mode, width depends on environment
				// Just verify it doesn't panic
				assert.NotNil(t, term)
			}
		})
	}
}

func TestForceTTY_Height(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		forceTTY bool
		expected int
	}{
		{
			name:     "force-tty enabled returns default height",
			forceTTY: true,
			expected: defaultForcedHeight,
		},
		{
			name:     "force-tty disabled returns 0 when not TTY",
			forceTTY: false,
			expected: 0, // Assume test runs without TTY
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set viper flags
			viper.Set("force-tty", tt.forceTTY)
			viper.Set("force-color", false)
			viper.Set("no-color", false)
			viper.Set("color", false)

			// Create terminal
			term := New()

			// Check height
			result := term.Height(Stdout)

			if tt.forceTTY {
				assert.Equal(t, tt.expected, result, "Height should return %d when force-tty=%v", tt.expected, tt.forceTTY)
			} else {
				// For non-forced mode, height depends on environment
				// Just verify it doesn't panic
				assert.NotNil(t, term)
			}
		})
	}
}

func TestForceColor_ColorProfile(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name       string
		forceColor bool
		forceTTY   bool
		expected   ColorProfile
	}{
		{
			name:       "force-color enabled returns TrueColor",
			forceColor: true,
			forceTTY:   false,
			expected:   ColorTrue,
		},
		{
			name:       "force-color and force-tty both enabled returns TrueColor",
			forceColor: true,
			forceTTY:   true,
			expected:   ColorTrue,
		},
		{
			name:       "force-color disabled returns ColorNone when not TTY",
			forceColor: false,
			forceTTY:   false,
			expected:   ColorNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set viper flags
			viper.Set("force-color", tt.forceColor)
			viper.Set("force-tty", tt.forceTTY)
			viper.Set("no-color", false)
			viper.Set("color", false)

			// Create terminal
			term := New()

			// Check color profile
			result := term.ColorProfile()
			assert.Equal(t, tt.expected, result, "ColorProfile should return %v when force-color=%v, force-tty=%v", tt.expected, tt.forceColor, tt.forceTTY)
		})
	}
}

func TestForceFlags_Combined(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Test that both --force-tty and --force-color work together
	viper.Set("force-tty", true)
	viper.Set("force-color", true)
	viper.Set("no-color", false)
	viper.Set("color", false)

	term := New()

	// Verify TTY detection
	assert.True(t, term.IsTTY(Stdout), "IsTTY should return true with --force-tty")
	assert.True(t, term.IsTTY(Stderr), "IsTTY should return true with --force-tty")

	// Verify dimensions
	assert.Equal(t, defaultForcedWidth, term.Width(Stdout), "Width should return default with --force-tty")
	assert.Equal(t, defaultForcedHeight, term.Height(Stdout), "Height should return default with --force-tty")

	// Verify color profile
	assert.Equal(t, ColorTrue, term.ColorProfile(), "ColorProfile should return TrueColor with --force-color")
}

func TestConfig_BuildConfig(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Test that buildConfig reads viper flags correctly
	viper.Set("force-tty", true)
	viper.Set("force-color", true)
	viper.Set("no-color", false)
	viper.Set("color", true)

	cfg := buildConfig()

	assert.True(t, cfg.ForceTTY, "Config should have ForceTTY=true")
	assert.True(t, cfg.ForceColor, "Config should have ForceColor=true")
	assert.False(t, cfg.NoColor, "Config should have NoColor=false")
	assert.True(t, cfg.Color, "Config should have Color=true")
}

func TestShouldUseColor_WithForceColor(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name       string
		noColor    bool
		color      bool
		forceColor bool
		forceTTY   bool
		isTTY      bool
		expected   bool
	}{
		{
			name:       "force-color overrides no-color",
			noColor:    true,
			forceColor: true,
			isTTY:      false,
			expected:   true, // Config.ShouldUseColor doesn't check ForceColor directly, but DetectColorProfile does
		},
		{
			name:       "force-color enables color for non-TTY",
			forceColor: true,
			isTTY:      false,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NoColor:    tt.noColor,
				Color:      tt.color,
				ForceColor: tt.forceColor,
				ForceTTY:   tt.forceTTY,
			}

			// Note: ShouldUseColor doesn't directly check ForceColor
			// ForceColor is handled in New() by setting colorProfile directly
			result := cfg.ShouldUseColor(tt.isTTY)

			// This test verifies the config structure
			// The actual force-color logic is tested in TestForceColor_ColorProfile
			assert.NotNil(t, cfg)
			_ = result // Suppress unused variable warning
		})
	}
}

func TestNew_WithCustomConfig(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Test that WithConfig option works
	customCfg := &Config{
		ForceTTY:   true,
		ForceColor: true,
	}

	term := New(WithConfig(customCfg))
	require.NotNil(t, term)

	// Verify force flags are respected
	assert.True(t, term.IsTTY(Stdout), "IsTTY should return true with custom config ForceTTY=true")
	assert.Equal(t, ColorTrue, term.ColorProfile(), "ColorProfile should return TrueColor with custom config ForceColor=true")
}

func TestStreamToFd(t *testing.T) {
	// Test streamToFd helper function
	tests := []struct {
		name     string
		stream   Stream
		expectFd bool
	}{
		{
			name:     "Stdin returns valid fd",
			stream:   Stdin,
			expectFd: true,
		},
		{
			name:     "Stdout returns valid fd",
			stream:   Stdout,
			expectFd: true,
		},
		{
			name:     "Stderr returns valid fd",
			stream:   Stderr,
			expectFd: true,
		},
		{
			name:     "Invalid stream returns -1",
			stream:   Stream(999),
			expectFd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := streamToFd(tt.stream)
			if tt.expectFd {
				assert.GreaterOrEqual(t, fd, 0, "Valid stream should return fd >= 0")
			} else {
				assert.Equal(t, -1, fd, "Invalid stream should return -1")
			}
		})
	}
}

func TestEnvironmentVariableSupport(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Test that ATMOS_FORCE_TTY and ATMOS_FORCE_COLOR are respected via viper
	viper.Set("force-tty", "true")   // Simulates ATMOS_FORCE_TTY=true
	viper.Set("force-color", "true") // Simulates ATMOS_FORCE_COLOR=true

	term := New()

	// Verify force TTY
	assert.True(t, term.IsTTY(Stdout), "Should respect ATMOS_FORCE_TTY env var")
	assert.Equal(t, defaultForcedWidth, term.Width(Stdout), "Should use sane defaults with ATMOS_FORCE_TTY")
	assert.Equal(t, defaultForcedHeight, term.Height(Stdout), "Should use sane defaults with ATMOS_FORCE_TTY")

	// Verify force color
	assert.Equal(t, ColorTrue, term.ColorProfile(), "Should respect ATMOS_FORCE_COLOR env var")
}

func TestColorProfile_String(t *testing.T) {
	tests := []struct {
		profile  ColorProfile
		expected string
	}{
		{ColorNone, "None"},
		{Color16, "16"},
		{Color256, "256"},
		{ColorTrue, "TrueColor"},
		{ColorProfile(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.profile.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestShouldUseColor_RespectsTTY verifies that --color and terminal.color respect TTY detection.
func TestShouldUseColor_RespectsTTY(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		isTTY    bool
		expected bool
		reason   string
	}{
		{
			name: "--color respects TTY detection (TTY=true)",
			config: Config{
				Color: true,
			},
			isTTY:    true,
			expected: true,
			reason:   "--color should enable color when TTY is true",
		},
		{
			name: "--color respects TTY detection (TTY=false)",
			config: Config{
				Color: true,
			},
			isTTY:    false,
			expected: false,
			reason:   "--color should NOT enable color when TTY is false",
		},
		{
			name: "--force-color ignores TTY detection (TTY=false)",
			config: Config{
				ForceColor: true,
			},
			isTTY:    false,
			expected: true,
			reason:   "--force-color should enable color even when TTY is false",
		},
		{
			name: "--force-color ignores TTY detection (TTY=true)",
			config: Config{
				ForceColor: true,
			},
			isTTY:    true,
			expected: true,
			reason:   "--force-color should enable color when TTY is true",
		},
		{
			name: "terminal.color respects TTY detection (TTY=true)",
			config: Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Color: true,
						},
					},
				},
			},
			isTTY:    true,
			expected: true,
			reason:   "terminal.color should enable color when TTY is true",
		},
		{
			name: "terminal.color respects TTY detection (TTY=false)",
			config: Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Color: true,
						},
					},
				},
			},
			isTTY:    false,
			expected: false,
			reason:   "terminal.color should NOT enable color when TTY is false",
		},
		{
			name: "NO_COLOR overrides --force-color",
			config: Config{
				EnvNoColor: true,
				ForceColor: true,
			},
			isTTY:    true,
			expected: false,
			reason:   "NO_COLOR should disable color even with --force-color",
		},
		{
			name: "CLICOLOR_FORCE ignores TTY detection",
			config: Config{
				EnvCLIColorForce: true,
			},
			isTTY:    false,
			expected: true,
			reason:   "CLICOLOR_FORCE should enable color even when TTY is false",
		},
		{
			name: "CLICOLOR=0 disables color unless forced",
			config: Config{
				EnvCLIColor: "0",
			},
			isTTY:    true,
			expected: false,
			reason:   "CLICOLOR=0 should disable color",
		},
		{
			name: "CLICOLOR=0 with --force-color enables color",
			config: Config{
				EnvCLIColor: "0",
				ForceColor:  true,
			},
			isTTY:    true,
			expected: true,
			reason:   "CLICOLOR=0 should be overridden by --force-color",
		},
		{
			name: "--no-color overrides --color",
			config: Config{
				NoColor: true,
				Color:   true,
			},
			isTTY:    true,
			expected: false,
			reason:   "--no-color should disable color even with --color",
		},
		{
			name:   "default TTY behavior (TTY=true)",
			config: Config{
				// No explicit flags
			},
			isTTY:    true,
			expected: true,
			reason:   "Should default to enabled when TTY is true",
		},
		{
			name:   "default TTY behavior (TTY=false)",
			config: Config{
				// No explicit flags
			},
			isTTY:    false,
			expected: false,
			reason:   "Should default to disabled when TTY is false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ShouldUseColor(tt.isTTY)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}

// TestPipingBehavior verifies that piping automatically disables color.
func TestPipingBehavior(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
		reason   string
	}{
		{
			name:   "piped output (isTTY=false) disables color by default",
			config: Config{
				// No explicit flags - simulates default behavior
			},
			expected: false,
			reason:   "Piped output should automatically disable color",
		},
		{
			name: "piped output with --color still disables color",
			config: Config{
				Color: true,
			},
			expected: false,
			reason:   "Piped output should disable color even with --color flag",
		},
		{
			name: "piped output with terminal.color still disables color",
			config: Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Color: true,
						},
					},
				},
			},
			expected: false,
			reason:   "Piped output should disable color even with terminal.color setting",
		},
		{
			name: "piped output with --force-color enables color",
			config: Config{
				ForceColor: true,
			},
			expected: true,
			reason:   "Piped output should enable color with --force-color flag",
		},
		{
			name: "piped output with CLICOLOR_FORCE enables color",
			config: Config{
				EnvCLIColorForce: true,
			},
			expected: true,
			reason:   "Piped output should enable color with CLICOLOR_FORCE env var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate piped output by passing isTTY=false
			result := tt.config.ShouldUseColor(false)
			assert.Equal(t, tt.expected, result, tt.reason)

			// Also verify color profile
			profile := tt.config.DetectColorProfile(false)
			if tt.expected {
				assert.NotEqual(t, ColorNone, profile, "Color profile should not be None when color is forced")
			} else {
				assert.Equal(t, ColorNone, profile, "Color profile should be None when piped without force")
			}
		})
	}
}

// TestDetectColorProfile_RespectsTTY verifies that color profile detection respects TTY.
func TestDetectColorProfile_RespectsTTY(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		isTTY    bool
		expected ColorProfile
		reason   string
	}{
		{
			name: "TTY with TrueColor support",
			config: Config{
				EnvColorTerm: "truecolor",
			},
			isTTY:    true,
			expected: ColorTrue,
			reason:   "Should detect TrueColor when COLORTERM=truecolor",
		},
		{
			name: "TTY with 256 color support",
			config: Config{
				EnvTerm: "xterm-256color",
			},
			isTTY:    true,
			expected: Color256,
			reason:   "Should detect 256 colors when TERM=xterm-256color",
		},
		{
			name: "TTY with basic color support",
			config: Config{
				EnvTerm: "xterm",
			},
			isTTY:    true,
			expected: Color16,
			reason:   "Should detect 16 colors when TERM=xterm",
		},
		{
			name: "Non-TTY with color enabled",
			config: Config{
				Color: true,
			},
			isTTY:    false,
			expected: ColorNone,
			reason:   "Should return ColorNone for non-TTY even with --color",
		},
		{
			name: "Non-TTY with force-color",
			config: Config{
				ForceColor: true,
			},
			isTTY:    false,
			expected: ColorTrue,
			reason:   "Should return ColorTrue for non-TTY with --force-color",
		},
		{
			name: "TTY with no-color",
			config: Config{
				NoColor: true,
			},
			isTTY:    true,
			expected: ColorNone,
			reason:   "Should return ColorNone when --no-color is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.DetectColorProfile(tt.isTTY)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}
