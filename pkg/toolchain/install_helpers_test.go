package toolchain

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSpinnerControl_StartStop tests the spinner control start/stop methods.
func TestSpinnerControl_StartStop(t *testing.T) {
	// Test with showingSpinner = false - should not panic.
	sc := &spinnerControl{
		showingSpinner: false,
		program:        nil,
	}

	assert.NotPanics(t, func() {
		sc.start("test message")
	})

	assert.NotPanics(t, func() {
		sc.stop()
	})
}

// TestSpinnerControl_Restart tests the spinner control restart method.
func TestSpinnerControl_Restart(t *testing.T) {
	// Test with showingSpinner = false - should not panic.
	sc := &spinnerControl{
		showingSpinner: false,
		program:        nil,
	}

	assert.NotPanics(t, func() {
		sc.restart("new message")
	})
}

// TestResolveLatestVersionWithSpinner tests the resolveLatestVersionWithSpinner function.
func TestResolveLatestVersionWithSpinner(t *testing.T) {
	// Test with non-latest version - should return the version as-is.
	spinner := &spinnerControl{
		showingSpinner: false,
	}

	tests := []struct {
		name     string
		owner    string
		repo     string
		version  string
		isLatest bool
		wantErr  bool
	}{
		{
			name:     "specific version not latest",
			owner:    "hashicorp",
			repo:     "terraform",
			version:  "1.11.4",
			isLatest: false,
			wantErr:  false,
		},
		{
			name:     "version with isLatest false",
			owner:    "hashicorp",
			repo:     "terraform",
			version:  "latest",
			isLatest: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveLatestVersionWithSpinner(tt.owner, tt.repo, tt.version, tt.isLatest, spinner)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Non-latest versions should return as-is.
				if !tt.isLatest || tt.version != "latest" {
					assert.Equal(t, tt.version, resolved)
				}
			}
		})
	}
}

// TestHandleInstallSuccess tests the handleInstallSuccess function.
func TestHandleInstallSuccess(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, ".atmos", "tools", "bin")

	// Create an installer using factory function.
	installer := NewInstallerWithBinDir(binDir)

	tests := []struct {
		name   string
		result installResult
	}{
		{
			name: "success with show message false",
			result: installResult{
				owner:                  "hashicorp",
				repo:                   "terraform",
				version:                "1.11.4",
				binaryPath:             filepath.Join(tempDir, ".atmos", "tools", "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
				isLatest:               false,
				showMessage:            false,
				showHint:               false,
				skipToolVersionsUpdate: true,
			},
		},
		{
			name: "success with show message true and skip tool versions",
			result: installResult{
				owner:                  "hashicorp",
				repo:                   "terraform",
				version:                "1.11.4",
				binaryPath:             filepath.Join(tempDir, ".atmos", "tools", "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
				isLatest:               false,
				showMessage:            true,
				showHint:               false,
				skipToolVersionsUpdate: true,
			},
		},
		{
			name: "success with show hint true",
			result: installResult{
				owner:                  "hashicorp",
				repo:                   "terraform",
				version:                "1.11.4",
				binaryPath:             filepath.Join(tempDir, ".atmos", "tools", "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
				isLatest:               false,
				showMessage:            true,
				showHint:               true,
				skipToolVersionsUpdate: true,
			},
		},
		{
			name: "success with is latest true",
			result: installResult{
				owner:                  "hashicorp",
				repo:                   "terraform",
				version:                "1.11.4",
				binaryPath:             filepath.Join(tempDir, ".atmos", "tools", "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
				isLatest:               true,
				showMessage:            false,
				showHint:               false,
				skipToolVersionsUpdate: true,
			},
		},
		{
			name: "success with all flags true",
			result: installResult{
				owner:                  "hashicorp",
				repo:                   "terraform",
				version:                "1.11.4",
				binaryPath:             filepath.Join(tempDir, ".atmos", "tools", "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
				isLatest:               true,
				showMessage:            true,
				showHint:               true,
				skipToolVersionsUpdate: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// handleInstallSuccess should not panic with various input combinations.
			assert.NotPanics(t, func() {
				handleInstallSuccess(tt.result, installer)
			})
		})
	}
}

// TestInstallResult_Fields tests that installResult struct fields work correctly.
func TestInstallResult_Fields(t *testing.T) {
	result := installResult{
		owner:                  "hashicorp",
		repo:                   "terraform",
		version:                "1.11.4",
		binaryPath:             "/path/to/terraform",
		isLatest:               true,
		showMessage:            true,
		showHint:               true,
		skipToolVersionsUpdate: true,
	}

	assert.Equal(t, "hashicorp", result.owner)
	assert.Equal(t, "terraform", result.repo)
	assert.Equal(t, "1.11.4", result.version)
	assert.Equal(t, "/path/to/terraform", result.binaryPath)
	assert.True(t, result.isLatest)
	assert.True(t, result.showMessage)
	assert.True(t, result.showHint)
	assert.True(t, result.skipToolVersionsUpdate)
}

// TestSpinnerControl_Fields tests that spinnerControl struct fields work correctly.
func TestSpinnerControl_Fields(t *testing.T) {
	sc := &spinnerControl{
		showingSpinner: true,
		program:        nil,
	}

	assert.True(t, sc.showingSpinner)
	assert.Nil(t, sc.program)
}

// TestGetPlatformPathHint tests that getPlatformPathHint returns the correct hint for the current platform.
func TestGetPlatformPathHint(t *testing.T) {
	hint := getPlatformPathHint()

	// Verify the hint is not empty.
	assert.NotEmpty(t, hint)

	// Verify the hint contains expected content based on platform.
	if runtime.GOOS == "windows" {
		assert.True(t, strings.Contains(hint, "Invoke-Expression"),
			"Windows hint should contain 'Invoke-Expression'")
		assert.True(t, strings.Contains(hint, "--format powershell"),
			"Windows hint should contain '--format powershell'")
	} else {
		assert.True(t, strings.Contains(hint, "eval"),
			"Unix hint should contain 'eval'")
		assert.True(t, strings.Contains(hint, "$("),
			"Unix hint should contain '$('")
	}

	// All platforms should include the atmos toolchain env command.
	assert.True(t, strings.Contains(hint, "atmos"),
		"Hint should contain 'atmos'")
	assert.True(t, strings.Contains(hint, "toolchain env"),
		"Hint should contain 'toolchain env'")
	assert.True(t, strings.Contains(hint, "PATH"),
		"Hint should contain 'PATH'")
}
