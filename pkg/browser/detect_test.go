package browser

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMockOsStat replaces the osStat function variable for the duration of the test.
func withMockOsStat(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()
	orig := osStat
	osStat = fn
	t.Cleanup(func() { osStat = orig })
}

// withMockExecLookPath replaces the execLookPath function variable for the duration of the test.
func withMockExecLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := execLookPath
	execLookPath = fn
	t.Cleanup(func() { execLookPath = orig })
}

// withMockUserCurrent replaces the userCurrent function variable for the duration of the test.
func withMockUserCurrent(t *testing.T, fn func() (*user.User, error)) {
	t.Helper()
	orig := userCurrent
	userCurrent = fn
	t.Cleanup(func() { userCurrent = orig })
}

// statAllowingPaths returns a mock osStat that succeeds for any path in the allowedPaths set.
func statAllowingPaths(allowedPaths map[string]bool) func(string) (os.FileInfo, error) {
	return func(name string) (os.FileInfo, error) {
		if allowedPaths[name] {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
}

func TestDetectChromeDarwin(t *testing.T) {
	tests := []struct {
		name             string
		allowedPaths     map[string]bool
		wantErr          bool
		wantPath         string
		wantAppName      string
		wantUseMacOSOpen bool
	}{
		{
			name: "finds Google Chrome first",
			allowedPaths: map[string]bool{
				"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome": true,
			},
			wantPath:         "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			wantAppName:      "Google Chrome",
			wantUseMacOSOpen: true,
		},
		{
			name: "finds Chromium when Chrome absent",
			allowedPaths: map[string]bool{
				"/Applications/Chromium.app/Contents/MacOS/Chromium": true,
			},
			wantPath:         "/Applications/Chromium.app/Contents/MacOS/Chromium",
			wantAppName:      "Chromium",
			wantUseMacOSOpen: true,
		},
		{
			name: "finds Canary when others absent",
			allowedPaths: map[string]bool{
				"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary": true,
			},
			wantPath:         "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			wantAppName:      "Google Chrome Canary",
			wantUseMacOSOpen: true,
		},
		{
			name: "prefers Google Chrome over Chromium",
			allowedPaths: map[string]bool{
				"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome": true,
				"/Applications/Chromium.app/Contents/MacOS/Chromium":           true,
			},
			wantPath:         "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			wantAppName:      "Google Chrome",
			wantUseMacOSOpen: true,
		},
		{
			name:         "returns error when none found",
			allowedPaths: map[string]bool{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockOsStat(t, statAllowingPaths(tt.allowedPaths))

			info, err := detectChromeDarwin()

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrChromeNotFound))
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.wantPath, info.Path)
			assert.Equal(t, tt.wantAppName, info.AppName)
			assert.Equal(t, tt.wantUseMacOSOpen, info.UseMacOSOpen)
		})
	}
}

func TestDetectChromeLinux(t *testing.T) {
	tests := []struct {
		name      string
		foundName string // which candidate to return success for ("" = none)
		foundPath string
		wantErr   bool
		wantPath  string
	}{
		{
			name:      "finds google-chrome",
			foundName: "google-chrome",
			foundPath: "/usr/bin/google-chrome",
			wantPath:  "/usr/bin/google-chrome",
		},
		{
			name:      "finds google-chrome-stable",
			foundName: "google-chrome-stable",
			foundPath: "/usr/bin/google-chrome-stable",
			wantPath:  "/usr/bin/google-chrome-stable",
		},
		{
			name:      "finds chromium-browser",
			foundName: "chromium-browser",
			foundPath: "/usr/bin/chromium-browser",
			wantPath:  "/usr/bin/chromium-browser",
		},
		{
			name:      "finds chromium as last fallback",
			foundName: "chromium",
			foundPath: "/usr/bin/chromium",
			wantPath:  "/usr/bin/chromium",
		},
		{
			name:    "returns error when none found",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockExecLookPath(t, func(name string) (string, error) {
				if name == tt.foundName {
					return tt.foundPath, nil
				}
				return "", errors.New("not found")
			})

			info, err := detectChromeLinux()

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrChromeNotFound))
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.wantPath, info.Path)
			assert.False(t, info.UseMacOSOpen)
			assert.Empty(t, info.AppName)
		})
	}
}

func TestDetectChromeWindows(t *testing.T) {
	programFilesPath := filepath.Join("C:", "Program Files", chromePath, chromeExe)
	programFilesX86Path := filepath.Join("C:", "Program Files (x86)", chromePath, chromeExe)

	tests := []struct {
		name         string
		allowedPaths map[string]bool
		userHome     string
		userErr      error
		lookPathName string // if set, execLookPath returns this for chromeExe
		wantErr      bool
		wantPath     string
	}{
		{
			name: "finds in Program Files",
			allowedPaths: map[string]bool{
				programFilesPath: true,
			},
			userHome: "/home/test",
			wantPath: programFilesPath,
		},
		{
			name: "finds in Program Files x86",
			allowedPaths: map[string]bool{
				programFilesX86Path: true,
			},
			userHome: "/home/test",
			wantPath: programFilesX86Path,
		},
		{
			name: "finds in LocalAppData",
			allowedPaths: map[string]bool{
				filepath.Join(string(filepath.Separator), "home", "test", "AppData", "Local", chromePath, chromeExe): true,
			},
			userHome: filepath.Join(string(filepath.Separator), "home", "test"),
			wantPath: filepath.Join(string(filepath.Separator), "home", "test", "AppData", "Local", chromePath, chromeExe),
		},
		{
			name:         "userCurrent error skips AppData, falls back to PATH",
			allowedPaths: map[string]bool{},
			userErr:      errors.New("user lookup failed"),
			lookPathName: "/usr/local/bin/chrome.exe",
			wantPath:     "/usr/local/bin/chrome.exe",
		},
		{
			name:         "PATH fallback when all osStat fail",
			allowedPaths: map[string]bool{},
			userHome:     "/home/test",
			lookPathName: "/found/in/path/chrome.exe",
			wantPath:     "/found/in/path/chrome.exe",
		},
		{
			name:         "returns error when nothing found",
			allowedPaths: map[string]bool{},
			userHome:     "/home/test",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockOsStat(t, statAllowingPaths(tt.allowedPaths))
			withMockUserCurrent(t, func() (*user.User, error) {
				if tt.userErr != nil {
					return nil, tt.userErr
				}
				return &user.User{HomeDir: tt.userHome}, nil
			})
			withMockExecLookPath(t, func(name string) (string, error) {
				if tt.lookPathName != "" && name == chromeExe {
					return tt.lookPathName, nil
				}
				return "", errors.New("not found")
			})

			info, err := detectChromeWindows()

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrChromeNotFound))
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.wantPath, info.Path)
			assert.False(t, info.UseMacOSOpen)
		})
	}
}

func TestDetectChrome(t *testing.T) {
	t.Run("returns result for current platform", func(t *testing.T) {
		// Mock both osStat and execLookPath to simulate Chrome being found.
		withMockOsStat(t, func(name string) (os.FileInfo, error) {
			// Succeed for any Chrome-like path.
			if strings.Contains(name, "Chrome") || strings.Contains(name, "Chromium") || strings.Contains(name, chromeExe) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		})
		withMockExecLookPath(t, func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		})

		info, err := DetectChrome()
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.NotEmpty(t, info.Path)

		// Verify platform-specific behavior.
		switch runtime.GOOS {
		case "darwin":
			assert.True(t, info.UseMacOSOpen)
			assert.NotEmpty(t, info.AppName)
		case "linux":
			assert.False(t, info.UseMacOSOpen)
		case "windows":
			assert.False(t, info.UseMacOSOpen)
		}
	})

	t.Run("returns ErrChromeNotFound when not installed", func(t *testing.T) {
		withMockOsStat(t, func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		})
		withMockExecLookPath(t, func(string) (string, error) {
			return "", errors.New("not found")
		})
		withMockUserCurrent(t, func() (*user.User, error) {
			return &user.User{HomeDir: "/nonexistent"}, nil
		})

		info, err := DetectChrome()
		require.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrChromeNotFound))
		assert.Nil(t, info)
	})
}
