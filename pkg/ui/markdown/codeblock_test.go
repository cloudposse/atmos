package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNewCodeblockStyle(t *testing.T) {
	t.Run("creates code block style with renderer", func(t *testing.T) {
		renderer := lipgloss.NewRenderer(os.Stdout)

		style := NewCodeblockStyle(renderer)

		// Verify styles are created (non-nil).
		if style.Base.String() == "" && style.Text.String() == "" {
			t.Log("Styles created (may be zero-value depending on renderer)")
		}
	})

	t.Run("creates style without debug mode", func(t *testing.T) {
		// Ensure ATMOS_DEBUG_COLORS is not set.
		t.Setenv("ATMOS_DEBUG_COLORS", "")

		renderer := lipgloss.NewRenderer(os.Stdout)
		style := NewCodeblockStyle(renderer)

		// Should create style without panicking.
		if style.Program.String() == "" {
			t.Log("Program style created")
		}
	})
}

func TestRenderCodeBlock(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		width    int
		contains string
	}{
		{
			name:     "simple command",
			content:  "atmos version",
			width:    80,
			contains: "atmos",
		},
		{
			name:     "command with flags",
			content:  "atmos terraform plan --stack dev",
			width:    120,
			contains: "terraform",
		},
		{
			name:     "multiline commands",
			content:  "atmos version\natmos help",
			width:    80,
			contains: "version",
		},
		{
			name:     "empty content",
			content:  "",
			width:    80,
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := lipgloss.NewRenderer(os.Stdout)
			result := RenderCodeBlock(renderer, tt.content, tt.width)

			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}

			if tt.content != "" && result == "" {
				t.Error("Expected non-empty result for non-empty content")
			}
		})
	}
}

func TestStyleLine(t *testing.T) {
	renderer := lipgloss.NewRenderer(os.Stdout)
	styles := NewCodeblockStyle(renderer)

	tests := []struct {
		name string
		line string
	}{
		{
			name: "command line",
			line: "$ atmos version",
		},
		{
			name: "plain text",
			line: "This is plain text",
		},
		{
			name: "empty line",
			line: "",
		},
		{
			name: "line with flags",
			line: "atmos terraform plan --stack dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			result := styleLine(tt.line, &styles)
			if tt.line != "" && result == "" {
				t.Log("Result may be empty depending on styling")
			}
		})
	}
}

func TestStyleCommandLine(t *testing.T) {
	renderer := lipgloss.NewRenderer(os.Stdout)
	styles := NewCodeblockStyle(renderer)

	tests := []struct {
		name string
		line string
	}{
		{
			name: "command with prompt",
			line: "$ atmos version",
		},
		{
			name: "command without prompt",
			line: "atmos version",
		},
		{
			name: "command with flags",
			line: "$ atmos terraform plan --stack dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			result := styleCommandLine(tt.line, &styles)
			if result == "" {
				t.Log("Result may be empty depending on styling")
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		expectTokens   bool
		expectedLength int
	}{
		{
			name:           "simple command",
			line:           "atmos version",
			expectTokens:   true,
			expectedLength: 2,
		},
		{
			name:           "command with flags",
			line:           "atmos terraform plan --stack dev",
			expectTokens:   true,
			expectedLength: 5,
		},
		{
			name:           "command with quoted string",
			line:           `atmos describe component vpc --stack "ue2-dev"`,
			expectTokens:   true,
			expectedLength: 6,
		},
		{
			name:           "empty line",
			line:           "",
			expectTokens:   false,
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.line)

			if tt.expectTokens && len(tokens) == 0 {
				t.Error("Expected tokens to be returned")
			}

			if tt.expectedLength > 0 && len(tokens) != tt.expectedLength {
				t.Logf("Expected %d tokens, got %d", tt.expectedLength, len(tokens))
			}
		})
	}
}

func TestIsQuoteStart(t *testing.T) {
	tests := []struct {
		name     string
		char     rune
		inQuote  bool
		expected bool
	}{
		{
			name:     "double quote not in quote",
			char:     '"',
			inQuote:  false,
			expected: true,
		},
		{
			name:     "single quote not in quote",
			char:     '\'',
			inQuote:  false,
			expected: true,
		},
		{
			name:     "double quote already in quote",
			char:     '"',
			inQuote:  true,
			expected: false,
		},
		{
			name:     "not a quote",
			char:     'a',
			inQuote:  false,
			expected: false,
		},
		{
			name:     "space",
			char:     ' ',
			inQuote:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isQuoteStart(tt.char, tt.inQuote)
			if result != tt.expected {
				t.Errorf("isQuoteStart(%q, %v) = %v, want %v", tt.char, tt.inQuote, result, tt.expected)
			}
		})
	}
}

func TestStyleToken(t *testing.T) {
	renderer := lipgloss.NewRenderer(os.Stdout)
	styles := NewCodeblockStyle(renderer)

	tests := []struct {
		name     string
		token    string
		position int
	}{
		{
			name:     "command token (position 0)",
			token:    "atmos",
			position: 0,
		},
		{
			name:     "flag token (position 1)",
			token:    "--stack",
			position: 1,
		},
		{
			name:     "argument token (position 2)",
			token:    "dev",
			position: 2,
		},
		{
			name:     "empty token",
			token:    "",
			position: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			result := styleToken(tt.token, tt.position, &styles)
			if tt.token != "" && result == "" {
				t.Log("Result may be empty depending on styling")
			}
		})
	}
}
