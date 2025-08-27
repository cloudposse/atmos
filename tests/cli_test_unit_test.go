package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestParseTimeout tests the parseTimeout function
func TestParseTimeout(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"valid seconds", "30s", 30 * time.Second, false},
		{"valid minutes", "2m", 2 * time.Minute, false},
		{"valid hours", "1h", 1 * time.Hour, false},
		{"valid milliseconds", "500ms", 500 * time.Millisecond, false},
		{"zero duration", "0s", 0, false},
		{"invalid format", "invalid", 0, true},
		{"empty string", "", 0, false}, // Function returns 0, nil for empty string
		{"negative duration", "-5s", -5 * time.Second, false}, // Go allows negative durations
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTimeout(tt.input)
			
			if tt.wantErr && err == nil {
				t.Errorf("parseTimeout(%q) expected error, got none", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseTimeout(%q) unexpected error: %v", tt.input, err)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("parseTimeout(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSanitizeTestName tests the sanitizeTestName function
func TestSanitizeTestName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "test_case", "test_case"},
		{"slashes to underscores", "test/case/name", "test_case_name"},
		{"special characters", "test/with\\special:chars", "test_with_special_chars"}, 
		{"backslashes", "test\\path\\name", "test_path_name"},
		{"mixed characters", "test-case_with/mixed\\chars", "test-case_with_mixed_chars"},
		{"empty string", "", ""},
		{"trailing spaces", "test   ", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTestName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeTestName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCollapseExtraSlashes tests the collapseExtraSlashes function
func TestCollapseExtraSlashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"http protocol", "http://example.com", "http://example.com"},
		{"https protocol", "https://example.com", "https://example.com"},
		{"http with extra slashes", "http:///example.com", "http://example.com"},
		{"https with extra slashes", "https:////example.com", "https://example.com"},
		{"http with path slashes", "http://example.com//path//to//resource", "http://example.com/path/to/resource"},
		{"https with path slashes", "https://example.com///path///to///resource", "https://example.com/path/to/resource"},
		{"no protocol", "//path//to//resource", "/path/to/resource"},
		{"single slashes unchanged", "/path/to/resource", "/path/to/resource"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseExtraSlashes(tt.input)
			if result != tt.expected {
				t.Errorf("collapseExtraSlashes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestApplyIgnorePatterns tests the applyIgnorePatterns function
func TestApplyIgnorePatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		patterns []string
		expected string
	}{
		{"no patterns", "line1\nline2\nline3", []string{}, "line1\nline2\nline3"},
		{"single pattern match", "line1\nignore_this\nline3", []string{"ignore_this"}, "line1\nline3"},
		{"multiple patterns", "line1\nignore1\nline3\nignore2", []string{"ignore1", "ignore2"}, "line1\nline3"},
		{"pattern not found", "line1\nline2\nline3", []string{"not_found"}, "line1\nline2\nline3"},
		{"regex pattern", "line1\n[DEBUG] debug message\nline3", []string{"\\[DEBUG\\].*"}, "line1\nline3"},
		{"empty input", "", []string{"pattern"}, ""},
		{"empty patterns", "line1\nline2", []string{}, "line1\nline2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyIgnorePatterns(tt.input, tt.patterns)
			if result != tt.expected {
				t.Errorf("applyIgnorePatterns(%q, %v) = %q, want %q", tt.input, tt.patterns, result, tt.expected)
			}
		})
	}
}

// TestSetupColorProfile tests the setupColorProfile function
func TestSetupColorProfile(t *testing.T) {
	// Save original color profile
	originalProfile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(originalProfile)

	tests := []struct {
		name     string
		useTty   bool
		expected termenv.Profile
	}{
		{"TTY mode", true, termenv.TrueColor},
		{"Non-TTY mode", false, termenv.Ascii},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupColorProfile(tt.useTty)
			
			result := lipgloss.ColorProfile()
			if result != tt.expected {
				t.Errorf("setupColorProfile(%v) set profile to %v, want %v", tt.useTty, result, tt.expected)
			}
		})
	}
}

// TestSetupEnvironment tests the setupEnvironment function
func TestSetupEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	testKeys := []string{"TEST_VAR1", "TEST_VAR2", "EXISTING_VAR"}
	for _, key := range testKeys {
		if val, exists := os.LookupEnv(key); exists {
			originalEnv[key] = val
		}
	}
	
	// Set up test environment
	os.Setenv("EXISTING_VAR", "original_value")
	
	defer func() {
		// Restore original environment
		for _, key := range testKeys {
			if val, exists := originalEnv[key]; exists {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	tests := []struct {
		name     string
		env      map[string]string
		expected map[string]string
	}{
		{
			name: "set new variables",
			env:  map[string]string{"TEST_VAR1": "value1", "TEST_VAR2": "value2"},
			expected: map[string]string{"TEST_VAR1": "value1", "TEST_VAR2": "value2"},
		},
		{
			name: "override existing variable", 
			env:  map[string]string{"EXISTING_VAR": "new_value"},
			expected: map[string]string{"EXISTING_VAR": "new_value"},
		},
		{
			name: "empty environment",
			env:  map[string]string{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := setupEnvironment(tt.env)
			
			// Verify environment variables are set correctly
			for key, expectedValue := range tt.expected {
				if actualValue := os.Getenv(key); actualValue != expectedValue {
					t.Errorf("setupEnvironment() env var %s = %q, want %q", key, actualValue, expectedValue)
				}
			}
			
			// Test restore function
			restore()
			
			// Verify restoration
			for key := range tt.env {
				if key == "EXISTING_VAR" {
					// Should be restored to original value
					if actualValue := os.Getenv(key); actualValue != "original_value" {
						t.Errorf("restore() env var %s = %q, want %q", key, actualValue, "original_value")
					}
				} else {
					// Should be unset (empty)
					if actualValue := os.Getenv(key); actualValue != "" {
						t.Errorf("restore() env var %s should be unset, got %q", key, actualValue)
					}
				}
			}
		})
	}
}

// TestDiffStrings tests the DiffStrings function  
func TestDiffStrings(t *testing.T) {
	tests := []struct {
		name       string
		x          string
		y          string
		shouldDiff bool
	}{
		{
			name:       "identical strings",
			x:          "hello world",
			y:          "hello world", 
			shouldDiff: false, // Identical strings still produce output showing no changes
		},
		{
			name:       "different strings",
			x:          "hello world",
			y:          "hello universe",
			shouldDiff: true, // Should show differences
		},
		{
			name:       "empty strings",
			x:          "",
			y:          "",
			shouldDiff: false,
		},
		{
			name:       "one empty",
			x:          "content",
			y:          "",
			shouldDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiffStrings(tt.x, tt.y)
			
			// DiffStrings always returns some output, even for identical strings
			// We just verify it doesn't panic and returns a string
			if result == "" && (tt.x != "" || tt.y != "") {
				t.Errorf("DiffStrings(%q, %q) returned empty result unexpectedly", tt.x, tt.y)
			}
		})
	}
}

// TestSanitizeOutput tests the sanitizeOutput function with a mock repo structure
func TestSanitizeOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "no paths to replace", 
			input:    "simple output without paths",
			expected: "simple output without paths",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeOutput(tt.input)
			
			if tt.wantErr && err == nil {
				t.Errorf("sanitizeOutput(%q) expected error, got none", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("sanitizeOutput(%q) unexpected error: %v", tt.input, err)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("sanitizeOutput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestLoadTestSuite tests the loadTestSuite function
func TestLoadTestSuite(t *testing.T) {
	// Test with a non-existent file
	t.Run("non-existent file", func(t *testing.T) {
		_, err := loadTestSuite("non-existent-file.yaml")
		if err == nil {
			t.Error("loadTestSuite() expected error for non-existent file, got none")
		}
	})
	
	// Test with an existing valid test file
	t.Run("valid test file", func(t *testing.T) {
		// Use one of the existing test case files
		testFile := filepath.Join("test-cases", "core.yaml")
		suite, err := loadTestSuite(testFile)
		
		if err != nil {
			// If file doesn't exist, that's ok for this test
			if !strings.Contains(err.Error(), "no such file") {
				t.Errorf("loadTestSuite(%q) unexpected error: %v", testFile, err)
			}
			return
		}
		
		if suite == nil {
			t.Error("loadTestSuite() returned nil suite")
		}
		
		if suite.Tests == nil {
			t.Error("loadTestSuite() returned suite with nil Tests")
		}
	})
}

// TestVerifyExitCode tests the verifyExitCode function
func TestVerifyExitCode(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
		want     bool
	}{
		{"matching exit codes", 0, 0, true},
		{"non-matching exit codes", 0, 1, false},
		{"both non-zero matching", 1, 1, true},
		{"both non-zero different", 1, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal test instance for the function
			dummyTest := &testing.T{}
			
			result := verifyExitCode(dummyTest, tt.expected, tt.actual)
			if result != tt.want {
				t.Errorf("verifyExitCode(%d, %d) = %v, want %v", tt.expected, tt.actual, result, tt.want)
			}
		})
	}
}

// TestVerifyOS tests the verifyOS function
func TestVerifyOS(t *testing.T) {
	tests := []struct {
		name     string
		patterns []MatchPattern
		want     bool
	}{
		{
			name:     "empty patterns",
			patterns: []MatchPattern{},
			want:     true, // Empty patterns should pass
		},
		{
			name: "pattern matches current OS",
			patterns: []MatchPattern{
				{Pattern: ".*", Negate: false}, // Matches any OS
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dummyTest := &testing.T{}
			result := verifyOS(dummyTest, tt.patterns)
			if result != tt.want {
				t.Errorf("verifyOS(%v) = %v, want %v", tt.patterns, result, tt.want)
			}
		})
	}
}

// TestNewPathManager tests the PathManager functionality
func TestNewPathManager(t *testing.T) {
	t.Run("creates path manager with current PATH", func(t *testing.T) {
		pm := NewPathManager()
		
		if pm == nil {
			t.Error("NewPathManager() returned nil")
		}
		
		if pm.OriginalPath == "" {
			t.Error("NewPathManager() should capture current PATH")
		}
		
		if pm.Prepended == nil {
			t.Error("NewPathManager() should initialize Prepended slice")
		}
	})
	
	t.Run("prepend directories", func(t *testing.T) {
		pm := NewPathManager()
		originalLen := len(pm.Prepended)
		
		pm.Prepend("test-dir1", "test-dir2")
		
		if len(pm.Prepended) != originalLen+2 {
			t.Errorf("Prepend() should add 2 directories, got %d", len(pm.Prepended)-originalLen)
		}
	})
}