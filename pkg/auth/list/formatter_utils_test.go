package list

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TestGetStatusIndicator tests the status indicator rendering for all status types.
func TestGetStatusIndicator(t *testing.T) {
	defer perf.Track(nil, "list.TestGetStatusIndicator")()

	tests := []struct {
		name          string
		status        authStatus
		wantColor     string // Expected color number for verification.
		wantCharacter string
		wantIndicator bool // Whether an indicator should be shown (not just space).
	}{
		{
			name:          "valid credentials",
			status:        authStatusValid,
			wantColor:     "10", // Green.
			wantCharacter: "●",
			wantIndicator: true,
		},
		{
			name:          "expiring credentials",
			status:        authStatusExpiring,
			wantColor:     "11", // Yellow.
			wantCharacter: "●",
			wantIndicator: true,
		},
		{
			name:          "expired credentials",
			status:        authStatusExpired,
			wantColor:     "9", // Red.
			wantCharacter: "●",
			wantIndicator: true,
		},
		{
			name:          "unknown status",
			status:        authStatusUnknown,
			wantColor:     "",
			wantCharacter: " ",
			wantIndicator: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusIndicator(tt.status)

			// Check if indicator is present vs space.
			if tt.wantIndicator {
				if got == " " {
					t.Errorf("getStatusIndicator(%v) = space, want colored indicator", tt.status)
				}
				// Verify the indicator contains the expected character.
				if !containsCharacter(got, tt.wantCharacter) {
					t.Errorf("getStatusIndicator(%v) does not contain expected character %q, got %q", tt.status, tt.wantCharacter, got)
				}
			} else {
				if got != " " {
					t.Errorf("getStatusIndicator(%v) = %q, want space", tt.status, got)
				}
			}

			// Note: We don't strictly check for ANSI codes here because lipgloss may not
			// render them in all test environments. The ColorMatching test below verifies
			// the exact output matches expected styled strings.
		})
	}
}

// TestGetStatusIndicator_ColorMatching verifies the exact colors match the version list command.
func TestGetStatusIndicator_ColorMatching(t *testing.T) {
	defer perf.Track(nil, "list.TestGetStatusIndicator_ColorMatching")()

	// These should match the colors used in cmd/version/formatters.go.
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	expectedGreen := greenStyle.Render("●")
	expectedYellow := yellowStyle.Render("●")
	expectedRed := redStyle.Render("●")

	tests := []struct {
		name     string
		status   authStatus
		expected string
	}{
		{"valid matches green", authStatusValid, expectedGreen},
		{"expiring matches yellow", authStatusExpiring, expectedYellow},
		{"expired matches red", authStatusExpired, expectedRed},
		{"unknown is space", authStatusUnknown, " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusIndicator(tt.status)
			if got != tt.expected {
				t.Errorf("getStatusIndicator(%v) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

// containsCharacter checks if a string contains a specific character (ignoring ANSI codes).
func containsCharacter(s, char string) bool {
	// Simple check - the character should be present in the string.
	for _, r := range s {
		if string(r) == char {
			return true
		}
	}
	return false
}
