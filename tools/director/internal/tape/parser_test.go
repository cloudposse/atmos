package tape

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCommands(t *testing.T) {
	// Create a temporary tape file for testing.
	content := `# Test tape file
Output test.gif
Output test.mp4
Output test.svg

Set Theme "default"
Set Width 1400

# Setup
Hide
Type "PS1='> '"
Enter
Ctrl+L
Show

# First command
Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 1s
Enter
Sleep 3s

# Comment line (should be marked as comment)
Type "# This is a comment"
Enter

# Second command with pipe
Type "atmos describe component vpc -s plat-dev-ue2 | head -20"
Enter
Sleep 2s

# Type without Enter (should not be captured)
Type "incomplete"
Sleep 1s

# Another command
Type "echo hello"
Enter
`

	tmpDir := t.TempDir()
	tapePath := filepath.Join(tmpDir, "test.tape")
	if err := os.WriteFile(tapePath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write test tape: %v", err)
	}

	commands, err := ParseCommands(tapePath)
	if err != nil {
		t.Fatalf("ParseCommands failed: %v", err)
	}

	// Should have 5 commands: PS1, terraform plan, comment, describe|head, echo.
	if len(commands) != 5 {
		t.Errorf("Expected 5 commands, got %d", len(commands))
		for i, cmd := range commands {
			t.Logf("  [%d] line=%d comment=%v text=%q", i, cmd.Line, cmd.IsComment, cmd.Text)
		}
	}

	// Verify specific commands.
	tests := []struct {
		index     int
		text      string
		isComment bool
	}{
		{0, "PS1='> '", false},
		{1, "atmos terraform plan vpc -s plat-dev-ue2", false},
		{2, "# This is a comment", true},
		{3, "atmos describe component vpc -s plat-dev-ue2 | head -20", false},
		{4, "echo hello", false},
	}

	for _, tc := range tests {
		if tc.index >= len(commands) {
			t.Errorf("Missing command at index %d", tc.index)
			continue
		}
		cmd := commands[tc.index]
		if cmd.Text != tc.text {
			t.Errorf("Command %d: expected text %q, got %q", tc.index, tc.text, cmd.Text)
		}
		if cmd.IsComment != tc.isComment {
			t.Errorf("Command %d: expected IsComment=%v, got %v", tc.index, tc.isComment, cmd.IsComment)
		}
	}
}

func TestFilterExecutable(t *testing.T) {
	commands := []Command{
		{Line: 1, Text: "echo hello", IsComment: false},
		{Line: 2, Text: "# comment", IsComment: true},
		{Line: 3, Text: "echo world", IsComment: false},
		{Line: 4, Text: "# another comment", IsComment: true},
	}

	executable := FilterExecutable(commands)

	if len(executable) != 2 {
		t.Errorf("Expected 2 executable commands, got %d", len(executable))
	}

	if executable[0].Text != "echo hello" {
		t.Errorf("Expected first executable to be 'echo hello', got %q", executable[0].Text)
	}

	if executable[1].Text != "echo world" {
		t.Errorf("Expected second executable to be 'echo world', got %q", executable[1].Text)
	}
}

func TestParseCommandsWithBackticks(t *testing.T) {
	// Test that backtick-quoted Type directives also work.
	content := "Type `echo hello`\nEnter\n"

	tmpDir := t.TempDir()
	tapePath := filepath.Join(tmpDir, "test.tape")
	if err := os.WriteFile(tapePath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write test tape: %v", err)
	}

	commands, err := ParseCommands(tapePath)
	if err != nil {
		t.Fatalf("ParseCommands failed: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(commands))
	}

	if commands[0].Text != "echo hello" {
		t.Errorf("Expected 'echo hello', got %q", commands[0].Text)
	}
}
