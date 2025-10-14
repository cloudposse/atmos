package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

//nolint:dupl // Test functions for truthy/falsy are intentionally similar with opposite logic.
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
			input:    "yes",
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

//nolint:dupl // Test functions for truthy/falsy are intentionally similar with opposite logic.
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
			input:    "no",
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
			Run:    func(cmd *cobra.Command, args []string) {}, // Need Run function to be available
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
					Run: func(cmd *cobra.Command, args []string) {}, // Make subcommand available
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
		if result != 5 { // Length of "short"
			t.Errorf("Expected 5 (ignoring hidden), got %d", result)
		}
	})
}
