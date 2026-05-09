package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/env"
)

func TestOutputEnvAsExport(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		contains    []string
		notContains []string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			contains: []string{},
		},
		{
			name: "basic variable",
			envVars: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			contains: []string{"export AWS_REGION='us-east-1'"},
		},
		{
			name: "escapes single quotes",
			envVars: map[string]string{
				"VAR": "it's a test",
			},
			contains: []string{"export VAR='it'\\''s a test'"},
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"Z_VAR": "z",
				"A_VAR": "a",
				"M_VAR": "m",
			},
			contains: []string{"A_VAR", "M_VAR", "Z_VAR"},
		},
		{
			name: "special characters",
			envVars: map[string]string{
				"VAR": "value with spaces",
			},
			contains: []string{"export VAR='value with spaces'"},
		},
		{
			name: "empty value",
			envVars: map[string]string{
				"EMPTY": "",
			},
			contains: []string{"export EMPTY=''"},
		},
		{
			name: "value with equals",
			envVars: map[string]string{
				"VAR": "key=value",
			},
			contains: []string{"export VAR='key=value'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := outputEnvAsExport(tt.envVars)
			require.NoError(t, err)

			// Restore stdout and read output.
			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected)
			}
		})
	}
}

func TestOutputEnvAsExport_Sorting(t *testing.T) {
	envVars := map[string]string{
		"Z_VAR": "z",
		"A_VAR": "a",
		"M_VAR": "m",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsExport(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify A comes before M, M comes before Z.
	aIndex := strings.Index(output, "A_VAR")
	mIndex := strings.Index(output, "M_VAR")
	zIndex := strings.Index(output, "Z_VAR")

	assert.Less(t, aIndex, mIndex, "A_VAR should come before M_VAR")
	assert.Less(t, mIndex, zIndex, "M_VAR should come before Z_VAR")
}

func TestOutputEnvAsDotenv(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		contains    []string
		notContains []string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			contains: []string{},
		},
		{
			name: "basic variable",
			envVars: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			contains:    []string{"AWS_REGION='us-east-1'"},
			notContains: []string{"export"},
		},
		{
			name: "escapes single quotes",
			envVars: map[string]string{
				"VAR": "it's a test",
			},
			contains: []string{"VAR='it'\\''s a test'"},
		},
		{
			name: "no export prefix",
			envVars: map[string]string{
				"VAR": "value",
			},
			contains:    []string{"VAR='value'"},
			notContains: []string{"export"},
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"Z_VAR": "z",
				"A_VAR": "a",
			},
			contains: []string{"A_VAR", "Z_VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := outputEnvAsDotenv(tt.envVars)
			require.NoError(t, err)

			// Restore stdout and read output.
			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected)
			}
		})
	}
}

func TestOutputEnvAsDotenv_Sorting(t *testing.T) {
	envVars := map[string]string{
		"Z_VAR": "z",
		"A_VAR": "a",
		"M_VAR": "m",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsDotenv(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify A comes before M, M comes before Z.
	aIndex := strings.Index(output, "A_VAR")
	mIndex := strings.Index(output, "M_VAR")
	zIndex := strings.Index(output, "Z_VAR")

	assert.Less(t, aIndex, mIndex, "A_VAR should come before M_VAR")
	assert.Less(t, mIndex, zIndex, "M_VAR should come before Z_VAR")
}

func TestAuthEnvCommand_Structure(t *testing.T) {
	assert.Equal(t, "env", authEnvCmd.Use)
	assert.NotEmpty(t, authEnvCmd.Short)
	assert.NotEmpty(t, authEnvCmd.Long)
	assert.NotNil(t, authEnvCmd.RunE)

	// Check format flag exists.
	formatFlag := authEnvCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "f", formatFlag.Shorthand)

	// Check login flag exists.
	loginFlag := authEnvCmd.Flags().Lookup("login")
	assert.NotNil(t, loginFlag)
}

func TestSupportedFormats(t *testing.T) {
	// Verify the supported formats constant includes all formats supported by env.Output().
	assert.Contains(t, SupportedFormats, "json")
	assert.Contains(t, SupportedFormats, "bash")
	assert.Contains(t, SupportedFormats, "dotenv")
	assert.Contains(t, SupportedFormats, "env")
	assert.Contains(t, SupportedFormats, "github")
	assert.Len(t, SupportedFormats, 5)
}

func TestFormatFlagName(t *testing.T) {
	assert.Equal(t, "format", FormatFlagName)
}

func TestEnvCommand_FlagCompletion(t *testing.T) {
	// Test that format flag has completion registered.
	cmd := authEnvCmd

	// Get the flag.
	flag := cmd.Flags().Lookup("format")
	assert.NotNil(t, flag)

	// The flag should exist and have a default.
	assert.Equal(t, "bash", flag.DefValue)
}

func TestEnvParser_Initialization(t *testing.T) {
	// envParser should be initialized in init().
	assert.NotNil(t, envParser)
}

func TestOutputEnvAsExport_MultipleQuotes(t *testing.T) {
	envVars := map[string]string{
		"VAR": "it's got 'multiple' quotes",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsExport(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Each single quote should be escaped.
	assert.Contains(t, output, "export VAR=")
	// Count the escape sequences.
	assert.Equal(t, 3, strings.Count(output, "'\\''"))
}

func TestBuildConfigAndStacksInfo_WithEnvCommand(t *testing.T) {
	// Test that BuildConfigAndStacksInfo works with env command context.
	cmd := &cobra.Command{Use: "env"}
	v := viper.New()

	// This shouldn't panic.
	assert.NotPanics(t, func() {
		_ = BuildConfigAndStacksInfo(cmd, v)
	})
}

// TestGitHubEnvAutoDetect verifies the GitHub Actions format reads $GITHUB_ENV
// when no --output-file is provided, and produces correct heredoc framing for
// multiline values. Ported from main's auth_env_test.go (PR #1984).
func TestGitHubEnvAutoDetect(t *testing.T) {
	t.Run("auto-detects GITHUB_ENV and appends", func(t *testing.T) {
		tempDir := t.TempDir()
		githubEnvFile := filepath.Join(tempDir, "github_env")

		// Write initial content to simulate existing GitHub Actions env vars.
		err := os.WriteFile(githubEnvFile, []byte("EXISTING=value\n"), 0o644)
		require.NoError(t, err)

		// Set GITHUB_ENV environment variable.
		t.Setenv("GITHUB_ENV", githubEnvFile)

		envVars := map[string]string{
			"NEW_VAR": "new-value",
		}

		// Simulate what the command does when --format=github and no --output-file.
		output := os.Getenv("GITHUB_ENV")
		require.NotEmpty(t, output, "GITHUB_ENV should be set")

		// Use unified env.Output() which handles format and file writing.
		err = env.Output(envVars, "github", output, env.WithFileMode(env.CredentialFileMode))
		require.NoError(t, err)

		content, err := os.ReadFile(githubEnvFile)
		require.NoError(t, err)

		// Verify it appended (not overwrote) and used correct format.
		assert.Equal(t, "EXISTING=value\nNEW_VAR=new-value\n", string(content))
	})

	t.Run("auto-detect with multiline values", func(t *testing.T) {
		tempDir := t.TempDir()
		githubEnvFile := filepath.Join(tempDir, "github_env")

		t.Setenv("GITHUB_ENV", githubEnvFile)

		envVars := map[string]string{
			"CERT": "-----BEGIN CERT-----\ndata\n-----END CERT-----",
		}

		output := os.Getenv("GITHUB_ENV")

		// Use unified env.Output() which handles format and file writing.
		err := env.Output(envVars, "github", output, env.WithFileMode(env.CredentialFileMode))
		require.NoError(t, err)

		content, err := os.ReadFile(githubEnvFile)
		require.NoError(t, err)

		// Verify heredoc format for multiline.
		assert.Contains(t, string(content), "CERT<<ATMOS_EOF_CERT")
		assert.Contains(t, string(content), "-----BEGIN CERT-----\ndata\n-----END CERT-----\n")
		assert.Contains(t, string(content), "ATMOS_EOF_CERT\n")
	})
}

// TestResolveEnvOutputFile covers all branches of the github auto-detect path.
func TestResolveEnvOutputFile(t *testing.T) {
	t.Run("non-github format passes outputFile through", func(t *testing.T) {
		got, err := resolveEnvOutputFile("bash", "/tmp/out.sh")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/out.sh", got)
	})

	t.Run("non-github format with empty outputFile returns empty", func(t *testing.T) {
		got, err := resolveEnvOutputFile("dotenv", "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("github format with explicit outputFile passes through", func(t *testing.T) {
		got, err := resolveEnvOutputFile("github", "/explicit/path")
		require.NoError(t, err)
		assert.Equal(t, "/explicit/path", got)
	})

	t.Run("github format with GITHUB_ENV env var auto-detects", func(t *testing.T) {
		tempDir := t.TempDir()
		envFile := filepath.Join(tempDir, "github_env")
		t.Setenv("GITHUB_ENV", envFile)

		got, err := resolveEnvOutputFile("github", "")
		require.NoError(t, err)
		assert.Equal(t, envFile, got)
	})

	t.Run("github format without outputFile or GITHUB_ENV errors", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "")

		got, err := resolveEnvOutputFile("github", "")
		require.Error(t, err)
		assert.Empty(t, got)
		// Sentinel must be ErrRequiredFlagNotProvided so callers can branch on it.
		assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	})
}

// TestResolveEnvOutputTarget covers the viper-backed orchestrator over
// resolveEnvOutputFile.
func TestResolveEnvOutputTarget(t *testing.T) {
	t.Run("default format is bash when viper has no value", func(t *testing.T) {
		v := viper.New()

		format, outputFile, err := resolveEnvOutputTarget(v)
		require.NoError(t, err)
		assert.Equal(t, "bash", format)
		assert.Empty(t, outputFile)
	})

	t.Run("explicit format and output-file in viper round-trip", func(t *testing.T) {
		v := viper.New()
		v.Set(FormatFlagName, "dotenv")
		v.Set(OutputFileFlagName, "/path/to/.env")

		format, outputFile, err := resolveEnvOutputTarget(v)
		require.NoError(t, err)
		assert.Equal(t, "dotenv", format)
		assert.Equal(t, "/path/to/.env", outputFile)
	})

	t.Run("github format auto-detects $GITHUB_ENV", func(t *testing.T) {
		tempDir := t.TempDir()
		envFile := filepath.Join(tempDir, "github_env")
		t.Setenv("GITHUB_ENV", envFile)

		v := viper.New()
		v.Set(FormatFlagName, "github")

		format, outputFile, err := resolveEnvOutputTarget(v)
		require.NoError(t, err)
		assert.Equal(t, "github", format)
		assert.Equal(t, envFile, outputFile)
	})

	t.Run("github format without GITHUB_ENV propagates error", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "")

		v := viper.New()
		v.Set(FormatFlagName, "github")

		_, _, err := resolveEnvOutputTarget(v)
		require.Error(t, err)
	})
}

// TestLoginIfNeeded covers the cache-hit, fresh-auth-success, ErrUserAborted,
// and generic-failure branches via a mocked AuthManager.
func TestLoginIfNeeded(t *testing.T) {
	ctx := context.Background()

	t.Run("cached credentials skip Authenticate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetCachedCredentials(ctx, "id").Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)
		// Authenticate must NOT be called when cache is warm.
		m.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, loginIfNeeded(ctx, m, "id"))
	})

	t.Run("missing cache triggers Authenticate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetCachedCredentials(ctx, "id").Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(ctx, "id").Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)

		require.NoError(t, loginIfNeeded(ctx, m, "id"))
	})

	t.Run("ErrUserAborted surfaces unwrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetCachedCredentials(ctx, "id").Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(ctx, "id").Return(nil, errUtils.ErrUserAborted)

		err := loginIfNeeded(ctx, m, "id")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUserAborted)
		// Must not be wrapped with ErrAuthenticationFailed (clean abort).
		assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})

	t.Run("generic Authenticate error is wrapped with ErrAuthenticationFailed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		boom := errors.New("backend down")
		m.EXPECT().GetCachedCredentials(ctx, "id").Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(ctx, "id").Return(nil, boom)

		err := loginIfNeeded(ctx, m, "id")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.ErrorIs(t, err, boom, "original error must be preserved in the chain")
	})
}

// TestResolveIdentityNameForEnv covers the flag, viper-fallback, and default
// auto-detection branches via a mocked AuthManager.
func TestResolveIdentityNameForEnv(t *testing.T) {
	t.Run("explicit --identity flag wins", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		// GetDefaultIdentity must NOT be called.
		m.EXPECT().GetDefaultIdentity(gomock.Any()).Times(0)

		cmd := &cobra.Command{Use: "env"}
		cmd.Flags().String(IdentityFlagName, "", "identity")
		require.NoError(t, cmd.Flags().Set(IdentityFlagName, "explicit-id"))

		v := viper.New()

		got, err := resolveIdentityNameForEnv(cmd, v, m)
		require.NoError(t, err)
		assert.Equal(t, "explicit-id", got)
	})

	t.Run("viper fallback when flag unset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetDefaultIdentity(gomock.Any()).Times(0)

		cmd := &cobra.Command{Use: "env"}
		cmd.Flags().String(IdentityFlagName, "", "identity")

		v := viper.New()
		v.Set(IdentityFlagName, "viper-id")

		got, err := resolveIdentityNameForEnv(cmd, v, m)
		require.NoError(t, err)
		assert.Equal(t, "viper-id", got)
	})

	t.Run("falls back to GetDefaultIdentity when neither set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetDefaultIdentity(false).Return("default-id", nil)

		cmd := &cobra.Command{Use: "env"}
		cmd.Flags().String(IdentityFlagName, "", "identity")
		v := viper.New()

		got, err := resolveIdentityNameForEnv(cmd, v, m)
		require.NoError(t, err)
		assert.Equal(t, "default-id", got)
	})

	t.Run("__SELECT__ value forces interactive selection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetDefaultIdentity(true).Return("picked-id", nil)

		cmd := &cobra.Command{Use: "env"}
		cmd.Flags().String(IdentityFlagName, "", "identity")
		require.NoError(t, cmd.Flags().Set(IdentityFlagName, IdentityFlagSelectValue))

		v := viper.New()

		got, err := resolveIdentityNameForEnv(cmd, v, m)
		require.NoError(t, err)
		assert.Equal(t, "picked-id", got)
	})
}
