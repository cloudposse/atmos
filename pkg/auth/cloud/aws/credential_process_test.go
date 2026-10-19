package aws

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// Compile-time guard so a rename of the credential fields used below fails the build.
var _ = types.AWSCredentials{AccessKeyID: "", SecretAccessKey: "", SessionToken: "", Expiration: ""}

const (
	testSecretValue    = "SUPERSECRETVALUE0123456789abcdefghij"
	testAccessKeyValue = "AKIATESTACCESSKEY0123"
	testSessionValue   = "SESSIONTOKENVALUE0123456789"
)

// processCapture records how the credential_process command builder was invoked.
type processCapture struct {
	calls   int
	command string
	env     []string
}

// builder returns a command builder that runs the test binary as a fake credential helper. The
// helper environment variables select the behavior (see runFakeCredentialProcess).
func (c *processCapture) builder(t *testing.T, helperEnv ...string) CredentialProcessCommandBuilder {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)

	return func(ctx context.Context, command string, env []string) (*exec.Cmd, error) {
		c.calls++
		c.command = command
		c.env = append([]string(nil), env...)

		cmd := exec.CommandContext(ctx, exe)
		cmd.Env = append(append([]string(nil), env...), helperEnv...)
		return cmd, nil
	}
}

// baseEnviron returns the parent environment to hand to the helper (needed on Windows for
// SYSTEMROOT and friends) without any inherited credential_process recursion chain.
func baseEnviron() []string {
	var environ []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, CredentialProcessChainEnvVar+"=") {
			continue
		}
		environ = append(environ, kv)
	}
	return environ
}

func jsonEnv(t *testing.T, doc any) string {
	t.Helper()
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return testCredentialProcessJSONEnv + "=" + string(data)
}

func retrieveWithHelper(t *testing.T, helperEnv []string, opts ...CredentialProcessOption) (*types.AWSCredentials, *processCapture, error) {
	t.Helper()

	capture := &processCapture{}
	all := append([]CredentialProcessOption{
		WithCredentialProcessCommandBuilder(capture.builder(t, helperEnv...)),
		WithCredentialProcessEnviron(baseEnviron()),
	}, opts...)
	creds, err := RetrieveProcessCredentials(context.Background(), "corp-base", "fake-helper --flag", all...)
	return creds, capture, err
}

func TestRetrieveProcessCredentials_ValidOutput(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
		want types.AWSCredentials
	}{
		{
			name: "long-lived credentials without session token or expiration",
			doc: map[string]any{
				"Version":         1,
				"AccessKeyId":     testAccessKeyValue,
				"SecretAccessKey": testSecretValue,
			},
			want: types.AWSCredentials{
				AccessKeyID:     testAccessKeyValue,
				SecretAccessKey: testSecretValue,
			},
		},
		{
			name: "session token without expiration",
			doc: map[string]any{
				"Version":         1,
				"AccessKeyId":     testAccessKeyValue,
				"SecretAccessKey": testSecretValue,
				"SessionToken":    testSessionValue,
			},
			want: types.AWSCredentials{
				AccessKeyID:     testAccessKeyValue,
				SecretAccessKey: testSecretValue,
				SessionToken:    testSessionValue,
			},
		},
		{
			name: "temporary credentials with UTC expiration",
			doc: map[string]any{
				"Version":         1,
				"AccessKeyId":     testAccessKeyValue,
				"SecretAccessKey": testSecretValue,
				"SessionToken":    testSessionValue,
				"Expiration":      "2099-01-02T03:04:05Z",
			},
			want: types.AWSCredentials{
				AccessKeyID:     testAccessKeyValue,
				SecretAccessKey: testSecretValue,
				SessionToken:    testSessionValue,
				Expiration:      "2099-01-02T03:04:05Z",
			},
		},
		{
			name: "expiration with offset is normalized to UTC RFC3339",
			doc: map[string]any{
				"Version":         1,
				"AccessKeyId":     testAccessKeyValue,
				"SecretAccessKey": testSecretValue,
				"SessionToken":    testSessionValue,
				"Expiration":      "2099-01-02T05:04:05+02:00",
			},
			want: types.AWSCredentials{
				AccessKeyID:     testAccessKeyValue,
				SecretAccessKey: testSecretValue,
				SessionToken:    testSessionValue,
				Expiration:      "2099-01-02T03:04:05Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, capture, err := retrieveWithHelper(t, []string{jsonEnv(t, tt.doc)})
			require.NoError(t, err)
			require.NotNil(t, creds)

			assert.Equal(t, tt.want.AccessKeyID, creds.AccessKeyID)
			assert.Equal(t, tt.want.SecretAccessKey, creds.SecretAccessKey)
			assert.Equal(t, tt.want.SessionToken, creds.SessionToken)
			assert.Equal(t, tt.want.Expiration, creds.Expiration)
			assert.Empty(t, creds.Region)
			assert.Equal(t, 1, capture.calls)
			assert.Equal(t, "fake-helper --flag", capture.command)
		})
	}
}

func TestRetrieveProcessCredentials_InvalidOutput(t *testing.T) {
	tests := []struct {
		name       string
		helperEnv  []string
		wantDetail string
	}{
		{
			name: "wrong version",
			helperEnv: []string{testCredentialProcessJSONEnv + `={"Version":2,"AccessKeyId":"` + testAccessKeyValue +
				`","SecretAccessKey":"` + testSecretValue + `"}`},
			wantDetail: "Version must be 1",
		},
		{
			name:       "missing access key id",
			helperEnv:  []string{testCredentialProcessJSONEnv + `={"Version":1,"SecretAccessKey":"` + testSecretValue + `"}`},
			wantDetail: "AccessKeyId is missing",
		},
		{
			name:       "missing secret access key",
			helperEnv:  []string{testCredentialProcessJSONEnv + `={"Version":1,"AccessKeyId":"` + testAccessKeyValue + `"}`},
			wantDetail: "SecretAccessKey is missing",
		},
		{
			name:       "invalid JSON that embeds a secret",
			helperEnv:  []string{testCredentialProcessJSONEnv + `={"Version":1,"SecretAccessKey":"` + testSecretValue + `"`},
			wantDetail: "not a valid process-credential JSON document",
		},
		{
			name: "non RFC3339 expiration that embeds a secret",
			helperEnv: []string{testCredentialProcessJSONEnv + `={"Version":1,"AccessKeyId":"` + testAccessKeyValue +
				`","SecretAccessKey":"` + testSecretValue + `","Expiration":"tomorrow"}`},
			wantDetail: "not a valid process-credential JSON document",
		},
		{
			name:       "empty stdout",
			helperEnv:  []string{testCredentialProcessExitEnv + "=0"},
			wantDetail: "printed nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, _, err := retrieveWithHelper(t, tt.helperEnv)
			require.Error(t, err)
			assert.Nil(t, creds)

			assert.ErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
			assert.NotErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
			assert.Contains(t, err.Error(), tt.wantDetail)
			assert.Contains(t, err.Error(), "corp-base")

			// Regression: the AWS SDK embeds raw stdout in its parse errors. None of the secret
			// material the helper printed may appear in the error text.
			for _, secret := range []string{testSecretValue, testAccessKeyValue, testSessionValue} {
				assert.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestRetrieveProcessCredentials_NonZeroExit(t *testing.T) {
	creds, _, err := retrieveWithHelper(t, []string{testCredentialProcessExitEnv + "=3"})
	require.Error(t, err)
	assert.Nil(t, creds)

	assert.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
	assert.Contains(t, err.Error(), "corp-base")
}

func TestRetrieveProcessCredentials_NonZeroExitWithSecretOnStdout(t *testing.T) {
	// A failing helper that also printed credentials must still be reported as a failure.
	doc := map[string]any{"Version": 1, "AccessKeyId": testAccessKeyValue, "SecretAccessKey": testSecretValue}
	creds, _, err := retrieveWithHelper(t, []string{jsonEnv(t, doc), testCredentialProcessExitEnv + "=1"})
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
}

func TestRetrieveProcessCredentials_Timeout(t *testing.T) {
	start := time.Now()
	creds, _, err := retrieveWithHelper(
		t,
		[]string{testCredentialProcessSleepEnv + "=30s", testCredentialProcessExitEnv + "=0"},
		WithCredentialProcessTimeout(500*time.Millisecond),
	)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
	assert.Less(t, elapsed, 25*time.Second, "the helper must be killed when the timeout expires")
}

func TestRetrieveProcessCredentials_BuilderError(t *testing.T) {
	builderErr := errors.New("cannot build command")
	creds, err := RetrieveProcessCredentials(
		context.Background(), "corp-base", "fake-helper",
		WithCredentialProcessCommandBuilder(func(context.Context, string, []string) (*exec.Cmd, error) {
			return nil, builderErr
		}),
		WithCredentialProcessEnviron(baseEnviron()),
	)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
	assert.ErrorIs(t, err, builderErr)
}

func TestRetrieveProcessCredentials_EmptyCommand(t *testing.T) {
	for _, command := range []string{"", "   ", "\t\n"} {
		capture := &processCapture{}
		creds, err := RetrieveProcessCredentials(
			context.Background(), "corp-base", command,
			WithCredentialProcessCommandBuilder(capture.builder(t)),
		)
		require.Error(t, err)
		assert.Nil(t, creds)
		assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
		assert.Equal(t, 0, capture.calls, "an empty command must never be executed")
	}
}

func TestRetrieveProcessCredentials_Options(t *testing.T) {
	t.Run("non-positive timeout keeps the default", func(t *testing.T) {
		doc := map[string]any{"Version": 1, "AccessKeyId": testAccessKeyValue, "SecretAccessKey": testSecretValue}
		creds, _, err := retrieveWithHelper(t, []string{jsonEnv(t, doc)}, WithCredentialProcessTimeout(0))
		require.NoError(t, err)
		assert.Equal(t, testAccessKeyValue, creds.AccessKeyID)
	})

	t.Run("nil builder keeps the default", func(t *testing.T) {
		opts := &credentialProcessOptions{builder: DefaultCredentialProcessCommandBuilder}
		WithCredentialProcessCommandBuilder(nil)(opts)
		assert.NotNil(t, opts.builder)
	})
}

func TestRetrieveProcessCredentials_RecursionGuard(t *testing.T) {
	validDoc := map[string]any{"Version": 1, "AccessKeyId": testAccessKeyValue, "SecretAccessKey": testSecretValue}
	chainEntry := func(chain string) string { return CredentialProcessChainEnvVar + "=" + chain }

	t.Run("fires when the identity is already resolving", func(t *testing.T) {
		capture := &processCapture{}
		creds, err := RetrieveProcessCredentials(
			context.Background(), "corp-base", "fake-helper",
			WithCredentialProcessCommandBuilder(capture.builder(t, jsonEnv(t, validDoc))),
			WithCredentialProcessEnviron(append(baseEnviron(), chainEntry("outer,corp-base"))),
		)
		require.Error(t, err)
		assert.Nil(t, creds)
		assert.ErrorIs(t, err, errUtils.ErrCredentialProcessRecursion)
		assert.Contains(t, err.Error(), "outer -> corp-base -> corp-base")
		assert.Equal(t, 0, capture.calls, "the command must not run when recursion is detected")
	})

	t.Run("fires for a single-entry chain with whitespace", func(t *testing.T) {
		_, err := RetrieveProcessCredentials(
			context.Background(), "corp-base", "fake-helper",
			WithCredentialProcessCommandBuilder(func(context.Context, string, []string) (*exec.Cmd, error) {
				return nil, errors.New("must not be reached")
			}),
			WithCredentialProcessEnviron([]string{chainEntry(" corp-base ")}),
		)
		assert.ErrorIs(t, err, errUtils.ErrCredentialProcessRecursion)
	})

	t.Run("does not fire for a different identity and appends to the chain", func(t *testing.T) {
		capture := &processCapture{}
		creds, err := RetrieveProcessCredentials(
			context.Background(), "other-identity", "fake-helper",
			WithCredentialProcessCommandBuilder(capture.builder(t, jsonEnv(t, validDoc))),
			WithCredentialProcessEnviron(append(baseEnviron(), chainEntry("outer,corp-base"), "KEEP_ME=1")),
		)
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, testAccessKeyValue, creds.AccessKeyID)

		var chainEntries []string
		for _, kv := range capture.env {
			if strings.HasPrefix(kv, CredentialProcessChainEnvVar+"=") {
				chainEntries = append(chainEntries, kv)
			}
		}
		assert.Equal(t, []string{chainEntry("outer,corp-base,other-identity")}, chainEntries,
			"the child must see exactly one chain variable, with the identity appended")
		assert.Contains(t, capture.env, "KEEP_ME=1", "unrelated environment must be passed through")
	})

	t.Run("starts a new chain when none exists", func(t *testing.T) {
		capture := &processCapture{}
		_, err := RetrieveProcessCredentials(
			context.Background(), "corp-base", "fake-helper",
			WithCredentialProcessCommandBuilder(capture.builder(t, jsonEnv(t, validDoc))),
			WithCredentialProcessEnviron(baseEnviron()),
		)
		require.NoError(t, err)
		assert.Contains(t, capture.env, chainEntry("corp-base"))
	})
}

func TestRetrieveProcessCredentials_DefaultEnvironIsProcessEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TEST_CREDENTIAL_PROCESS_MARKER", "present")

	capture := &processCapture{}
	_, err := RetrieveProcessCredentials(
		context.Background(), "corp-base", "fake-helper",
		WithCredentialProcessCommandBuilder(capture.builder(t,
			jsonEnv(t, map[string]any{"Version": 1, "AccessKeyId": testAccessKeyValue, "SecretAccessKey": testSecretValue}))),
	)
	require.NoError(t, err)
	assert.Contains(t, capture.env, "ATMOS_TEST_CREDENTIAL_PROCESS_MARKER=present")
}

// quoteCommandPath quotes an executable path so it survives both `sh -c` and `cmd.exe /C`.
func quoteCommandPath(path string) string {
	if runtime.GOOS == "windows" && !strings.ContainsAny(path, " \t") {
		return path
	}
	if runtime.GOOS == "windows" {
		return `"` + path + `"`
	}
	return `'` + strings.ReplaceAll(path, `'`, `'\''`) + `'`
}

// TestRetrieveProcessCredentials_DefaultCommandBuilder exercises the real shell path
// (`sh -c` / `cmd.exe /C`) end to end, using the test binary as the helper command.
func TestRetrieveProcessCredentials_DefaultCommandBuilder(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	doc := map[string]any{
		"Version":         1,
		"AccessKeyId":     testAccessKeyValue,
		"SecretAccessKey": testSecretValue,
		"SessionToken":    testSessionValue,
		"Expiration":      "2099-01-02T03:04:05Z",
	}

	creds, err := RetrieveProcessCredentials(
		context.Background(), "corp-base", quoteCommandPath(exe),
		WithCredentialProcessEnviron(append(baseEnviron(), jsonEnv(t, doc))),
	)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, testAccessKeyValue, creds.AccessKeyID)
	assert.Equal(t, testSecretValue, creds.SecretAccessKey)
	assert.Equal(t, testSessionValue, creds.SessionToken)
	assert.Equal(t, "2099-01-02T03:04:05Z", creds.Expiration)
}

func TestDefaultCredentialProcessCommandBuilder(t *testing.T) {
	env := []string{"A=1", "B=2"}
	cmd, err := DefaultCredentialProcessCommandBuilder(context.Background(), "do-something --now", env)
	require.NoError(t, err)
	require.NotNil(t, cmd)

	// The shell invocation itself is platform-specific (`sh -c` vs a verbatim cmd.exe command
	// line), so it is asserted in the *_unix_test.go / *_windows_test.go companions.
	assertDefaultBuilderShellInvocation(t, cmd, "do-something --now")
	assert.Equal(t, env, cmd.Env)
	assert.Equal(t, os.Stdin, cmd.Stdin, "stdin must be inherited so helpers can prompt for MFA")
	assert.Equal(t, os.Stderr, cmd.Stderr, "stderr must be inherited so helpers can print instructions")
	assert.Nil(t, cmd.Stdout, "stdout is captured by the caller")
}

func TestNewProcessCredentials(t *testing.T) {
	t.Run("long-lived credentials omit optional fields", func(t *testing.T) {
		out, err := NewProcessCredentials(&types.AWSCredentials{
			AccessKeyID:     testAccessKeyValue,
			SecretAccessKey: testSecretValue,
		})
		require.NoError(t, err)
		assert.Equal(t, &ProcessCredentials{
			Version:         CredentialProcessVersion,
			AccessKeyID:     testAccessKeyValue,
			SecretAccessKey: testSecretValue,
		}, out)

		data, err := MarshalProcessCredentials(&types.AWSCredentials{
			AccessKeyID:     testAccessKeyValue,
			SecretAccessKey: testSecretValue,
		})
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, map[string]any{
			"Version":         float64(1),
			"AccessKeyId":     testAccessKeyValue,
			"SecretAccessKey": testSecretValue,
		}, decoded, "SessionToken and Expiration must be omitted, not null or empty")
	})

	t.Run("temporary credentials include session token and UTC expiration", func(t *testing.T) {
		data, err := MarshalProcessCredentials(&types.AWSCredentials{
			AccessKeyID:     testAccessKeyValue,
			SecretAccessKey: testSecretValue,
			SessionToken:    testSessionValue,
			Expiration:      "2099-01-02T05:04:05+02:00",
		})
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, map[string]any{
			"Version":         float64(1),
			"AccessKeyId":     testAccessKeyValue,
			"SecretAccessKey": testSecretValue,
			"SessionToken":    testSessionValue,
			"Expiration":      "2099-01-02T03:04:05Z",
		}, decoded)
	})

	t.Run("rejects missing credentials", func(t *testing.T) {
		tests := map[string]struct {
			creds   *types.AWSCredentials
			wantErr error
		}{
			"nil":               {creds: nil, wantErr: errUtils.ErrIdentityCredentialsNone},
			"empty access key":  {creds: &types.AWSCredentials{SecretAccessKey: testSecretValue}, wantErr: errUtils.ErrAWSCredentialsIncomplete},
			"empty secret key":  {creds: &types.AWSCredentials{AccessKeyID: testAccessKeyValue}, wantErr: errUtils.ErrAWSCredentialsIncomplete},
			"both keys missing": {creds: &types.AWSCredentials{SessionToken: testSessionValue}, wantErr: errUtils.ErrAWSCredentialsIncomplete},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				out, err := NewProcessCredentials(tt.creds)
				require.ErrorIs(t, err, tt.wantErr)
				// Incomplete AWS credentials are still AWS credentials.
				assert.NotErrorIs(t, err, errUtils.ErrIdentityNotAWS)
				assert.Nil(t, out)

				data, err := MarshalProcessCredentials(tt.creds)
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, data)
			})
		}
	})

	t.Run("rejects an unparsable expiration", func(t *testing.T) {
		out, err := NewProcessCredentials(&types.AWSCredentials{
			AccessKeyID:     testAccessKeyValue,
			SecretAccessKey: testSecretValue,
			Expiration:      "not-a-time",
		})
		require.Error(t, err)
		assert.Nil(t, out)
		assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	})
}

// TestProcessCredentials_RoundTrip verifies the producer format is accepted by the consumer.
func TestProcessCredentials_RoundTrip(t *testing.T) {
	original := &types.AWSCredentials{
		AccessKeyID:     testAccessKeyValue,
		SecretAccessKey: testSecretValue,
		SessionToken:    testSessionValue,
		Expiration:      "2099-01-02T03:04:05Z",
	}
	data, err := MarshalProcessCredentials(original)
	require.NoError(t, err)

	creds, _, err := retrieveWithHelper(t, []string{testCredentialProcessJSONEnv + "=" + string(data)})
	require.NoError(t, err)
	assert.Equal(t, original, creds)
}
