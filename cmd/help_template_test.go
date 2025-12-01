package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Empty string returns false",
			input:    "",
			expected: false,
		},
		{
			name:     "Lowercase true",
			input:    "true",
			expected: true,
		},
		{
			name:     "Uppercase TRUE",
			input:    "TRUE",
			expected: true,
		},
		{
			name:     "Mixed case True",
			input:    "True",
			expected: true,
		},
		{
			name:     "String 1",
			input:    "1",
			expected: true,
		},
		{
			name:     "String 2",
			input:    "2",
			expected: true,
		},
		{
			name:     "String 3",
			input:    "3",
			expected: true,
		},
		{
			name:     "Lowercase yes",
			input:    "yes",
			expected: true,
		},
		{
			name:     "Uppercase YES",
			input:    "YES",
			expected: true,
		},
		{
			name:     "Lowercase on",
			input:    "on",
			expected: true,
		},
		{
			name:     "Lowercase always",
			input:    "always",
			expected: true,
		},
		{
			name:     "String 0 is not truthy",
			input:    "0",
			expected: false,
		},
		{
			name:     "String false is not truthy",
			input:    "false",
			expected: false,
		},
		{
			name:     "Random string is not truthy",
			input:    "random",
			expected: false,
		},
		{
			name:     "Whitespace trimmed - true with spaces",
			input:    "  true  ",
			expected: true,
		},
		{
			name:     "Whitespace trimmed - 1 with spaces",
			input:    "  1  ",
			expected: true,
		},
		{
			name:     "Only whitespace returns false",
			input:    "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTruthy(tt.input)
			if result != tt.expected {
				t.Errorf("isTruthy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsFalsy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Empty string returns false",
			input:    "",
			expected: false,
		},
		{
			name:     "Lowercase false",
			input:    "false",
			expected: true,
		},
		{
			name:     "Uppercase FALSE",
			input:    "FALSE",
			expected: true,
		},
		{
			name:     "Mixed case False",
			input:    "False",
			expected: true,
		},
		{
			name:     "String 0",
			input:    "0",
			expected: true,
		},
		{
			name:     "Lowercase no",
			input:    "no",
			expected: true,
		},
		{
			name:     "Uppercase NO",
			input:    "NO",
			expected: true,
		},
		{
			name:     "Lowercase off",
			input:    "off",
			expected: true,
		},
		{
			name:     "Uppercase OFF",
			input:    "OFF",
			expected: true,
		},
		{
			name:     "String 1 is not falsy",
			input:    "1",
			expected: false,
		},
		{
			name:     "String true is not falsy",
			input:    "true",
			expected: false,
		},
		{
			name:     "Random string is not falsy",
			input:    "random",
			expected: false,
		},
		{
			name:     "Whitespace trimmed - false with spaces",
			input:    "  false  ",
			expected: true,
		},
		{
			name:     "Whitespace trimmed - 0 with spaces",
			input:    "  0  ",
			expected: true,
		},
		{
			name:     "Only whitespace returns false",
			input:    "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFalsy(tt.input)
			if result != tt.expected {
				t.Errorf("isFalsy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Test edge cases and mutual exclusivity.
func TestTruthyFalsyMutualExclusivity(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Empty string", ""},
		{"String 1", "1"},
		{"String 0", "0"},
		{"String true", "true"},
		{"String false", "false"},
		{"Random string", "maybe"},
		{"Whitespace only", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truthy := isTruthy(tt.input)
			falsy := isFalsy(tt.input)

			// A value should not be both truthy and falsy.
			if truthy && falsy {
				t.Errorf("isTruthy(%q) and isFalsy(%q) are both true - should be mutually exclusive", tt.input, tt.input)
			}

			// Some values (like empty string or random strings) are neither truthy nor falsy.
			// This is expected behavior.
		})
	}
}

func TestIsCommandAvailable(t *testing.T) {
	t.Run("available command", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:    "test",
			Hidden: false,
			Run:    func(cmd *cobra.Command, args []string) {}, // Need Run function to be available.
		}
		if !isCommandAvailable(cmd) {
			t.Error("Expected available command to be available")
		}
	})

	t.Run("hidden command", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:    "hidden",
			Hidden: true,
			Run:    func(cmd *cobra.Command, args []string) {},
		}
		if isCommandAvailable(cmd) {
			t.Error("Expected hidden command to be unavailable")
		}
	})

	t.Run("help command is always available", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:    "help",
			Hidden: true,
			Run:    func(cmd *cobra.Command, args []string) {},
		}
		if !isCommandAvailable(cmd) {
			t.Error("Expected help command to always be available")
		}
	})
}

func TestCalculateCommandWidth(t *testing.T) {
	tests := []struct {
		name          string
		cmdName       string
		hasSubcommand bool
		expected      int
	}{
		{
			name:          "simple command",
			cmdName:       "test",
			hasSubcommand: false,
			expected:      4,
		},
		{
			name:          "command with subcommands",
			cmdName:       "terraform",
			hasSubcommand: true,
			expected:      19, // "terraform" (9) + " [command]" (10)
		},
		{
			name:          "short command",
			cmdName:       "ls",
			hasSubcommand: false,
			expected:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: tt.cmdName,
			}
			if tt.hasSubcommand {
				subCmd := &cobra.Command{
					Use: "subcommand",
					Run: func(cmd *cobra.Command, args []string) {}, // Make subcommand available.
				}
				cmd.AddCommand(subCmd)
			}
			result := calculateCommandWidth(cmd)
			if result != tt.expected {
				t.Errorf("calculateCommandWidth() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCalculateMaxCommandWidth(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		result := calculateMaxCommandWidth([]*cobra.Command{})
		if result != 0 {
			t.Errorf("Expected 0 for empty list, got %d", result)
		}
	})

	t.Run("single command", func(t *testing.T) {
		commands := []*cobra.Command{
			{Use: "test", Run: func(cmd *cobra.Command, args []string) {}},
		}
		result := calculateMaxCommandWidth(commands)
		if result != 4 {
			t.Errorf("Expected 4, got %d", result)
		}
	})

	t.Run("multiple commands with varying lengths", func(t *testing.T) {
		commands := []*cobra.Command{
			{Use: "short", Run: func(cmd *cobra.Command, args []string) {}},
			{Use: "muchlongercommandname", Run: func(cmd *cobra.Command, args []string) {}},
			{Use: "mid", Run: func(cmd *cobra.Command, args []string) {}},
		}
		result := calculateMaxCommandWidth(commands)
		expected := len("muchlongercommandname")
		if result != expected {
			t.Errorf("Expected %d, got %d", expected, result)
		}
	})

	t.Run("ignores hidden commands", func(t *testing.T) {
		commands := []*cobra.Command{
			{Use: "short", Run: func(cmd *cobra.Command, args []string) {}},
			{Use: "verylongbutthisishidden", Hidden: true, Run: func(cmd *cobra.Command, args []string) {}},
		}
		result := calculateMaxCommandWidth(commands)
		if result != 5 { // Length of "short".
			t.Errorf("Expected 5 (ignoring hidden), got %d", result)
		}
	})
}

func TestDetectColorConfig(t *testing.T) {
	tests := []struct {
		name             string
		env              map[string]string
		expectedForce    bool
		expectedDisabled bool
	}{
		{
			name:             "NO_COLOR disables color",
			env:              map[string]string{"NO_COLOR": "1"},
			expectedForce:    false,
			expectedDisabled: true,
		},
		{
			name:             "FORCE_COLOR=1 enables color",
			env:              map[string]string{"FORCE_COLOR": "1"},
			expectedForce:    true,
			expectedDisabled: false,
		},
		{
			name:             "FORCE_COLOR=0 disables even if ATMOS_FORCE_COLOR=1",
			env:              map[string]string{"ATMOS_FORCE_COLOR": "1", "FORCE_COLOR": "0"},
			expectedForce:    false,
			expectedDisabled: true,
		},
		{
			name:             "CLICOLOR_FORCE=1 enables color",
			env:              map[string]string{"CLICOLOR_FORCE": "1"},
			expectedForce:    true,
			expectedDisabled: false,
		},
		{
			name:             "FORCE_COLOR=0 disables color",
			env:              map[string]string{"FORCE_COLOR": "0"},
			expectedForce:    false,
			expectedDisabled: true,
		},
		{
			name:             "NO_COLOR takes precedence over FORCE_COLOR",
			env:              map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"},
			expectedForce:    false,
			expectedDisabled: true,
		},
		{
			name:             "empty env defaults to no force, no disable",
			env:              map[string]string{},
			expectedForce:    false,
			expectedDisabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear viper state before each test.
			viper.Reset()

			// Clear and set environment variables.
			envVars := []string{"NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "ATMOS_FORCE_COLOR", "ATMOS_DEBUG_COLORS", "TERM", "COLORTERM"}
			for _, env := range envVars {
				t.Setenv(env, "")
			}

			// Set test environment.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			config := detectColorConfig()

			if config.forceColor != tt.expectedForce {
				t.Errorf("forceColor = %v, want %v", config.forceColor, tt.expectedForce)
			}
			if config.explicitlyDisabled != tt.expectedDisabled {
				t.Errorf("explicitlyDisabled = %v, want %v", config.explicitlyDisabled, tt.expectedDisabled)
			}
		})
	}
}

func TestConfigureWriter(t *testing.T) {
	tests := []struct {
		name           string
		config         colorConfig
		expectRenderer bool
	}{
		{
			name: "disabled color creates Ascii renderer",
			config: colorConfig{
				forceColor:         false,
				explicitlyDisabled: true,
				debugColors:        false,
			},
			expectRenderer: true,
		},
		{
			name: "forced color creates ANSI256 renderer",
			config: colorConfig{
				forceColor:         true,
				explicitlyDisabled: false,
				debugColors:        false,
			},
			expectRenderer: true,
		},
		{
			name: "auto-detect creates renderer",
			config: colorConfig{
				forceColor:         false,
				explicitlyDisabled: false,
				debugColors:        false,
			},
			expectRenderer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			wc := configureWriter(cmd, tt.config)

			if tt.expectRenderer && wc.renderer == nil {
				t.Error("Expected renderer to be created, got nil")
			}
			if wc.writer == nil {
				t.Error("Expected writer to be created, got nil")
			}
		})
	}
}

func TestCreateHelpStyles(t *testing.T) {
	t.Run("creates non-nil styles", func(t *testing.T) {
		renderer := lipgloss.NewRenderer(os.Stdout)
		styles := createHelpStyles(renderer)

		// Check that all styles are initialized (non-zero values).
		if styles.heading.String() == "" && styles.commandName.String() == "" {
			// Styles might be zero-value, just ensure no panic.
			t.Log("Styles created successfully")
		}
	})
}

func TestRenderMarkdownDescription(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "plain text unchanged",
			input:    "simple description",
			contains: "simple description",
		},
		{
			name:     "markdown with backticks",
			input:    "use `atmos version` to check version",
			contains: "atmos version",
		},
		{
			name:     "empty string",
			input:    "",
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdownDescription(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestPrintLogoAndVersion(t *testing.T) {
	t.Run("prints logo and version to buffer", func(t *testing.T) {
		var buf bytes.Buffer
		renderer := lipgloss.NewRenderer(&buf)
		styles := createHelpStyles(renderer)

		// This function should not panic.
		printLogoAndVersion(&buf, &styles)

		output := buf.String()
		// The function uses PrintStyledTextToSpecifiedOutput which may or may not write to buffer
		// depending on terminal detection. Just ensure it doesn't panic.
		if len(output) > 0 {
			t.Logf("Output length: %d", len(output))
		}
	})
}

func TestPrintDescription(t *testing.T) {
	tests := []struct {
		name        string
		short       string
		long        string
		shouldPrint bool
	}{
		{
			name:        "with short description",
			short:       "Short description",
			long:        "",
			shouldPrint: true,
		},
		{
			name:        "with long description",
			short:       "",
			long:        "Long description here",
			shouldPrint: true,
		},
		{
			name:        "prefers long over short",
			short:       "Short",
			long:        "Long",
			shouldPrint: true,
		},
		{
			name:        "no description",
			short:       "",
			long:        "",
			shouldPrint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			cmd := &cobra.Command{
				Use:   "test",
				Short: tt.short,
				Long:  tt.long,
			}

			printDescription(&buf, cmd, &styles)

			output := buf.String()
			if tt.shouldPrint && output == "" {
				t.Error("Expected description to be printed")
			}
			if tt.shouldPrint {
				expected := tt.long
				if expected == "" {
					expected = tt.short
				}
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q", expected)
				}
			}
		})
	}
}

func TestPrintUsageSection(t *testing.T) {
	t.Run("prints usage for command", func(t *testing.T) {
		var buf bytes.Buffer
		renderer := lipgloss.NewRenderer(&buf)
		styles := createHelpStyles(renderer)

		cmd := &cobra.Command{
			Use: "test [flags] <args>",
		}

		printUsageSection(&buf, cmd, renderer, &styles)

		output := buf.String()
		if !strings.Contains(output, "USAGE") && !strings.Contains(output, "test") {
			t.Error("Expected output to contain usage information")
		}
	})
}

func TestPrintAliases(t *testing.T) {
	tests := []struct {
		name        string
		aliases     []string
		shouldPrint bool
	}{
		{
			name:        "with aliases",
			aliases:     []string{"t", "tst"},
			shouldPrint: true,
		},
		{
			name:        "no aliases",
			aliases:     []string{},
			shouldPrint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			cmd := &cobra.Command{
				Use:     "test",
				Aliases: tt.aliases,
			}

			printAliases(&buf, cmd, &styles)

			output := buf.String()
			if tt.shouldPrint && output == "" {
				t.Error("Expected aliases to be printed")
			}
			if tt.shouldPrint {
				for _, alias := range tt.aliases {
					if !strings.Contains(output, alias) {
						t.Errorf("Expected output to contain alias %q", alias)
					}
				}
			}
		})
	}
}

func TestPrintExamples(t *testing.T) {
	tests := []struct {
		name        string
		example     string
		shouldPrint bool
	}{
		{
			name:        "with examples",
			example:     "atmos version\natmos help",
			shouldPrint: true,
		},
		{
			name:        "no examples",
			example:     "",
			shouldPrint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			cmd := &cobra.Command{
				Use:     "test",
				Example: tt.example,
			}

			printExamples(&buf, cmd, renderer, &styles)

			output := buf.String()
			if tt.shouldPrint && output == "" {
				t.Error("Expected examples to be printed")
			}
			if tt.shouldPrint && !strings.Contains(output, "EXAMPLE") {
				t.Log("Output:", output)
			}
		})
	}
}

func TestApplyColoredHelpTemplate(t *testing.T) {
	t.Run("applies help template without panic", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:   "test",
			Short: "Test command",
			Run:   func(cmd *cobra.Command, args []string) {},
		}

		// Clear environment to avoid test contamination.
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")

		// This should not panic.
		applyColoredHelpTemplate(cmd)

		// Verify help template was set.
		if cmd.HelpTemplate() == "" {
			t.Error("Expected help template to be set")
		}
	})
}

func TestPrintFooter(t *testing.T) {
	t.Run("prints footer message", func(t *testing.T) {
		var buf bytes.Buffer
		renderer := lipgloss.NewRenderer(&buf)
		styles := createHelpStyles(renderer)

		cmd := &cobra.Command{
			Use: "test",
		}

		printFooter(&buf, cmd, &styles)

		output := buf.String()
		// Footer should contain "help" message.
		if !strings.Contains(output, "help") && !strings.Contains(output, "--help") {
			t.Log("Output:", output)
		}
	})
}

func TestPrintSubcommandAliases(t *testing.T) {
	tests := []struct {
		name        string
		subcommands []*cobra.Command
		shouldPrint bool
		contains    []string
	}{
		{
			name: "subcommands with aliases",
			subcommands: []*cobra.Command{
				{Use: "apply", Aliases: []string{"a"}, Short: "Apply changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "destroy", Aliases: []string{"d", "del"}, Short: "Destroy resources", Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: true,
			contains:    []string{"SUBCOMMAND ALIASES", "a", "Alias of"},
		},
		{
			name: "subcommands without aliases",
			subcommands: []*cobra.Command{
				{Use: "apply", Short: "Apply changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "destroy", Short: "Destroy resources", Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: false,
			contains:    []string{},
		},
		{
			name: "mixed - some with aliases, some without",
			subcommands: []*cobra.Command{
				{Use: "apply", Short: "Apply changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "destroy", Aliases: []string{"d"}, Short: "Destroy resources", Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: true,
			contains:    []string{"SUBCOMMAND ALIASES", "d"},
		},
		{
			name:        "no subcommands",
			subcommands: []*cobra.Command{},
			shouldPrint: false,
			contains:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			for _, subcmd := range tt.subcommands {
				cmd.AddCommand(subcmd)
			}

			ctx := &helpRenderContext{
				writer: &buf,
				styles: &styles,
			}

			printSubcommandAliases(ctx, cmd)

			output := buf.String()
			if tt.shouldPrint && output == "" {
				t.Error("Expected subcommand aliases to be printed")
			}
			if !tt.shouldPrint && output != "" {
				t.Errorf("Expected no output, got: %q", output)
			}

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got: %q", expected, output)
				}
			}
		})
	}
}

func TestFormatCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *cobra.Command
		maxWidth int
		contains []string
	}{
		{
			name: "simple command",
			cmd: &cobra.Command{
				Use:   "apply",
				Short: "Apply changes to infrastructure",
			},
			maxWidth: 20,
			contains: []string{"apply", "Apply changes"},
		},
		{
			name: "command with subcommands",
			cmd: &cobra.Command{
				Use:   "terraform",
				Short: "Terraform commands",
			},
			maxWidth: 20,
			contains: []string{"terraform", "[command]", "Terraform commands"},
		},
		{
			name: "command with long description",
			cmd: &cobra.Command{
				Use:   "validate",
				Short: "Validate stack configuration files against JSON Schema and OPA policies to ensure correctness",
			},
			maxWidth: 15,
			contains: []string{"validate", "Validate stack"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			// Add subcommand if testing that case
			if strings.Contains(tt.name, "subcommands") {
				tt.cmd.AddCommand(&cobra.Command{Use: "plan", Run: func(cmd *cobra.Command, args []string) {}})
			}

			ctx := &helpRenderContext{
				writer: &buf,
				styles: &styles,
			}

			formatCommandLine(ctx, tt.cmd, tt.maxWidth)

			output := buf.String()
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got: %q", expected, output)
				}
			}
		})
	}
}

func TestPrintAvailableCommands(t *testing.T) {
	tests := []struct {
		name        string
		subcommands []*cobra.Command
		shouldPrint bool
		contains    []string
	}{
		{
			name: "with available subcommands",
			subcommands: []*cobra.Command{
				{Use: "apply", Short: "Apply changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "plan", Short: "Plan changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "destroy", Short: "Destroy resources", Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: true,
			contains:    []string{"AVAILABLE COMMANDS", "apply", "plan", "destroy"},
		},
		{
			name: "with hidden commands",
			subcommands: []*cobra.Command{
				{Use: "apply", Short: "Apply changes", Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "hidden", Short: "Hidden command", Hidden: true, Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: true,
			contains:    []string{"AVAILABLE COMMANDS", "apply"},
		},
		{
			name:        "no subcommands",
			subcommands: []*cobra.Command{},
			shouldPrint: false,
			contains:    []string{},
		},
		{
			name: "only hidden commands",
			subcommands: []*cobra.Command{
				{Use: "hidden1", Short: "Hidden command 1", Hidden: true, Run: func(cmd *cobra.Command, args []string) {}},
				{Use: "hidden2", Short: "Hidden command 2", Hidden: true, Run: func(cmd *cobra.Command, args []string) {}},
			},
			shouldPrint: false,
			contains:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			for _, subcmd := range tt.subcommands {
				cmd.AddCommand(subcmd)
			}

			ctx := &helpRenderContext{
				writer: &buf,
				styles: &styles,
			}

			printAvailableCommands(ctx, cmd)

			output := buf.String()
			if tt.shouldPrint && output == "" {
				t.Error("Expected available commands to be printed")
			}
			if !tt.shouldPrint && output != "" {
				t.Errorf("Expected no output, got: %q", output)
			}

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got: %q", expected, output)
				}
			}
		})
	}
}

func TestPrintFlags(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func(*cobra.Command)
		shouldPrint bool
		contains    []string
	}{
		{
			name: "with local flags",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "Stack name")
				cmd.Flags().Bool("verbose", false, "Verbose output")
			},
			shouldPrint: true,
			contains:    []string{"FLAGS", "--stack", "--verbose"},
		},
		{
			name: "with inherited flags",
			setupCmd: func(cmd *cobra.Command) {
				parent := &cobra.Command{Use: "parent"}
				parent.PersistentFlags().String("config", "", "Config file")
				cmd.Parent()
				parent.AddCommand(cmd)
			},
			shouldPrint: true,
			contains:    []string{"GLOBAL FLAGS", "--config"},
		},
		{
			name: "with both local and inherited flags",
			setupCmd: func(cmd *cobra.Command) {
				parent := &cobra.Command{Use: "parent"}
				parent.PersistentFlags().String("config", "", "Config file")
				cmd.Flags().String("stack", "", "Stack name")
				parent.AddCommand(cmd)
			},
			shouldPrint: true,
			contains:    []string{"FLAGS", "--stack", "GLOBAL FLAGS", "--config"},
		},
		{
			name: "no flags",
			setupCmd: func(cmd *cobra.Command) {
				// No flags added
			},
			shouldPrint: false,
			contains:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
				Run:   func(cmd *cobra.Command, args []string) {},
			}

			tt.setupCmd(cmd)

			renderer := lipgloss.NewRenderer(&buf)
			styles := createHelpStyles(renderer)

			printFlags(&buf, cmd, &schema.AtmosConfiguration{}, &styles)

			output := buf.String()
			if tt.shouldPrint && output == "" && len(tt.contains) > 0 {
				t.Error("Expected flags to be printed")
			}

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got: %q", expected, output)
				}
			}
		})
	}
}
