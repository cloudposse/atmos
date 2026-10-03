package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

const (
	// CredentialProcessVersion is the only process-credential payload version defined by AWS.
	CredentialProcessVersion = 1

	// CredentialProcessChainEnvVar carries the comma-separated list of Atmos identities whose
	// credential_process is currently executing. It is propagated to every child process so a
	// helper that (directly or transitively) calls back into Atmos for the same identity is
	// detected instead of recursing forever.
	CredentialProcessChainEnvVar = "ATMOS_AUTH_CREDENTIAL_PROCESS_CHAIN"

	// DefaultCredentialProcessTimeout matches the AWS SDK default for credential_process.
	DefaultCredentialProcessTimeout = processcreds.DefaultTimeout

	// The wait delay bounds how long Wait blocks for the helper's stdout pipe to drain after the
	// helper was killed (timeout or cancellation). The shell runs the helper, so killing it leaves
	// grandchildren (for example `sleep 70`) holding the inherited pipe open; without a bound the
	// timeout would only be honored once those grandchildren exit on their own.
	credentialProcessWaitDelay = 2 * time.Second

	credentialProcessChainSeparator = ","
)

// ProcessCredentials is the JSON document defined by the AWS SDKs for `credential_process`.
// See https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html.
// Optional fields are omitted when empty so long-lived credentials never emit a null or
// empty Expiration, which some SDKs reject.
type ProcessCredentials struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken,omitempty"`
	Expiration      string `json:"Expiration,omitempty"`
}

// NewProcessCredentials converts Atmos AWS credentials into the AWS process-credential format.
func NewProcessCredentials(creds *types.AWSCredentials) (*ProcessCredentials, error) {
	defer perf.Track(nil, "aws.NewProcessCredentials")()

	if creds == nil {
		return nil, errUtils.ErrIdentityCredentialsNone
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, errUtils.ErrAWSCredentialsIncomplete
	}

	out := &ProcessCredentials{
		Version:         CredentialProcessVersion,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
	}

	if creds.Expiration != "" {
		expTime, err := time.Parse(time.RFC3339, creds.Expiration)
		if err != nil {
			return nil, fmt.Errorf("%w: failed parsing credential expiration: %w", errUtils.ErrInvalidAuthConfig, err)
		}
		out.Expiration = expTime.UTC().Format(time.RFC3339)
	}

	return out, nil
}

// MarshalProcessCredentials renders AWS credentials as compact process-credential JSON.
func MarshalProcessCredentials(creds *types.AWSCredentials) ([]byte, error) {
	defer perf.Track(nil, "aws.MarshalProcessCredentials")()

	out, err := NewProcessCredentials(creds)
	if err != nil {
		return nil, err
	}
	// Emitting the secret fields is the purpose of the process-credential contract.
	return json.Marshal(out) //nolint:gosec // G117: AWS credential_process output must contain SessionToken.
}

// CredentialProcessCommandBuilder builds the command that runs a credential_process string.
// The returned command must not have Stdout set; output is captured by the caller.
type CredentialProcessCommandBuilder func(ctx context.Context, command string, env []string) (*exec.Cmd, error)

type credentialProcessOptions struct {
	timeout time.Duration
	builder CredentialProcessCommandBuilder
	environ []string
}

// CredentialProcessOption configures RetrieveProcessCredentials.
type CredentialProcessOption func(*credentialProcessOptions)

// WithCredentialProcessTimeout overrides the default execution timeout.
func WithCredentialProcessTimeout(timeout time.Duration) CredentialProcessOption {
	return func(o *credentialProcessOptions) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// WithCredentialProcessCommandBuilder overrides how the command is built (used by tests).
func WithCredentialProcessCommandBuilder(builder CredentialProcessCommandBuilder) CredentialProcessOption {
	return func(o *credentialProcessOptions) {
		if builder != nil {
			o.builder = builder
		}
	}
}

// WithCredentialProcessEnviron overrides the base environment passed to the command
// (defaults to os.Environ()).
func WithCredentialProcessEnviron(environ []string) CredentialProcessOption {
	return func(o *credentialProcessOptions) {
		o.environ = environ
	}
}

// DefaultCredentialProcessCommandBuilder runs the command through the platform shell like the
// AWS SDKs do (`sh -c` on Unix, `cmd.exe /S /C` on Windows with the command line passed
// verbatim so quoted paths survive), so a command copied from ~/.aws/config behaves the same.
// Stdin and stderr are inherited so helpers can prompt for MFA or print browser-login
// instructions.
func DefaultCredentialProcessCommandBuilder(ctx context.Context, command string, env []string) (*exec.Cmd, error) {
	defer perf.Track(nil, "aws.DefaultCredentialProcessCommandBuilder")()

	cmd := process.NewShellCommand(ctx, command)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = credentialProcessWaitDelay
	return cmd, nil
}

// RetrieveProcessCredentials executes an AWS `credential_process` command on behalf of the
// given Atmos identity and returns the credentials it prints. Execution and payload
// validation are delegated to the AWS SDK's processcreds provider.
//
// Error text never includes the command's stdout: the SDK embeds raw output in its parse
// errors, and that output contains secrets.
func RetrieveProcessCredentials(ctx context.Context, identityName, command string, opts ...CredentialProcessOption) (*types.AWSCredentials, error) {
	defer perf.Track(nil, "aws.RetrieveProcessCredentials")()

	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("%w: credential_process is empty for identity %q", errUtils.ErrInvalidIdentityConfig, identityName)
	}

	options := credentialProcessOptions{
		timeout: DefaultCredentialProcessTimeout,
		builder: DefaultCredentialProcessCommandBuilder,
		environ: os.Environ(),
	}
	for _, opt := range opts {
		opt(&options)
	}

	childEnv, err := credentialProcessChildEnv(options.environ, identityName)
	if err != nil {
		return nil, err
	}

	// Capture the command so we can tell an execution failure apart from a payload that the
	// SDK rejected, and so we can describe invalid payloads without echoing them.
	var (
		ranCmd *exec.Cmd
		stdout bytes.Buffer
	)
	builder := processcreds.NewCommandBuilderFunc(func(ctx context.Context) (*exec.Cmd, error) {
		cmd, buildErr := options.builder(ctx, command, childEnv)
		if buildErr != nil {
			return nil, buildErr
		}
		cmd.Stdout = &stdout
		// Custom builders get the same bound so a timeout is honored promptly.
		if cmd.WaitDelay == 0 {
			cmd.WaitDelay = credentialProcessWaitDelay
		}
		ranCmd = cmd
		return cmd, nil
	})

	provider := processcreds.NewProviderCommand(builder, func(o *processcreds.Options) {
		o.Timeout = options.timeout
	})

	sdkCreds, err := provider.Retrieve(ctx)
	if err != nil {
		return nil, classifyProcessError(identityName, ranCmd, stdout.Bytes(), err)
	}

	creds := &types.AWSCredentials{
		AccessKeyID:     sdkCreds.AccessKeyID,
		SecretAccessKey: sdkCreds.SecretAccessKey,
		SessionToken:    sdkCreds.SessionToken,
	}
	if sdkCreds.CanExpire {
		creds.Expiration = sdkCreds.Expires.UTC().Format(time.RFC3339)
	}
	return creds, nil
}

// classifyProcessError turns the SDK error into either an invalid-output error (the helper ran and
// exited successfully, so the SDK rejected the payload) or an execution failure.
func classifyProcessError(identityName string, ranCmd *exec.Cmd, stdout []byte, err error) error {
	if ranCmd != nil && ranCmd.ProcessState != nil && ranCmd.ProcessState.Success() {
		// Do not wrap the SDK error: it embeds the raw helper output, which contains secrets.
		return NewInvalidProcessOutputError(identityName, describeInvalidProcessOutput(stdout))
	}
	return errUtils.Build(errUtils.ErrCredentialProcessFailed).
		WithCause(fmt.Errorf("identity %q: %w", identityName, err)).
		WithHintf("Run the credential_process command configured for identity %q manually in a terminal to see its error output", identityName).
		WithContext("identity", identityName).
		Err()
}

// NewInvalidProcessOutputError reports that the helper printed something other than a valid
// version-1 process-credential document. The detail must describe the problem without echoing
// any of the helper's output, which contains secrets.
func NewInvalidProcessOutputError(identityName, detail string) error {
	return errUtils.Build(errUtils.ErrCredentialProcessInvalidOutput).
		WithCausef("identity %q: %s", identityName, detail).
		WithHint("The command must print a version 1 process-credential JSON document to stdout (Version, AccessKeyId, SecretAccessKey, and optionally SessionToken and Expiration) and send diagnostics to stderr").
		WithHintf("Run the credential_process command configured for identity %q manually in a terminal and compare its stdout with the AWS process-credential format", identityName).
		WithContext("identity", identityName).
		Err()
}

// credentialProcessChildEnv returns the environment for the child process with the identity
// appended to the recursion-guard chain. It fails if the identity is already executing.
func credentialProcessChildEnv(environ []string, identityName string) ([]string, error) {
	prefix := CredentialProcessChainEnvVar + "="
	var chain []string
	env := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if value, ok := strings.CutPrefix(kv, prefix); ok {
			for _, name := range strings.Split(value, credentialProcessChainSeparator) {
				if name = strings.TrimSpace(name); name != "" {
					chain = append(chain, name)
				}
			}
			continue
		}
		env = append(env, kv)
	}

	if slices.Contains(chain, identityName) {
		return nil, errUtils.Build(errUtils.ErrCredentialProcessRecursion).
			WithCausef("identity %q is already resolving its credential_process (chain: %s); the command must not request credentials for the same identity",
				identityName, strings.Join(append(chain, identityName), " -> ")).
			WithHintf("Point the helper at a different identity, or remove the call back into Atmos for identity %q from its credential_process command", identityName).
			WithContext("identity", identityName).
			Err()
	}

	chain = append(chain, identityName)
	return append(env, prefix+strings.Join(chain, credentialProcessChainSeparator)), nil
}

// describeInvalidProcessOutput explains why a payload was rejected without including any of
// its values.
func describeInvalidProcessOutput(out []byte) string {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return "the command printed nothing to stdout"
	}

	var resp processcreds.CredentialProcessResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return "stdout is not a valid process-credential JSON document (or Expiration is not an RFC3339 timestamp)"
	}

	var problems []string
	if resp.Version != CredentialProcessVersion {
		problems = append(problems, fmt.Sprintf("Version must be %d", CredentialProcessVersion))
	}
	if resp.AccessKeyID == "" {
		problems = append(problems, "AccessKeyId is missing")
	}
	if resp.SecretAccessKey == "" {
		problems = append(problems, "SecretAccessKey is missing")
	}
	if len(problems) == 0 {
		return "the AWS SDK rejected the process-credential JSON document"
	}
	return strings.Join(problems, "; ")
}
