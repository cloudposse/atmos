package docker

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_credential_helper_test.go -package=$GOPACKAGE

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// credentialHelperPrefix is the executable name prefix Docker uses for credential helpers.
	credentialHelperPrefix = "docker-credential-"
	// credentialHelperTimeout bounds a single credential helper invocation. It is generous
	// because helpers such as osxkeychain or desktop may wait on an OS keychain prompt.
	credentialHelperTimeout = 60 * time.Second
	// Credential helper protocol actions (github.com/docker/docker-credential-helpers).
	helperActionStore = "store"
	helperActionErase = "erase"
	// helperNotFoundMessage is the protocol's standard "no such credential" reply.
	helperNotFoundMessage = "credentials not found in native keychain"
)

// CredentialHelper stores and erases registry credentials through a Docker
// credential helper (the docker-credential-<name> programs referenced by the
// credsStore and credHelpers keys of config.json).
type CredentialHelper interface {
	// Store saves username/secret for serverURL in the named helper.
	Store(helper, serverURL, username, secret string) error
	// Erase removes the credentials for serverURL from the named helper.
	// A missing credential is not an error.
	Erase(helper, serverURL string) error
}

// helperCredentials is the JSON payload of the credential helper "store" action.
// Field names are fixed by the docker-credential-helpers protocol.
type helperCredentials struct {
	ServerURL string `json:"ServerURL"`
	Username  string `json:"Username"`
	Secret    string `json:"Secret"`
}

// execCredentialHelper runs docker-credential-<name> executables using the
// docker-credential-helpers protocol, as docker/cli does
// (cli/config/credentials/native_store.go).
type execCredentialHelper struct {
	// command maps a helper name to the executable to run. Overridable in tests.
	command func(helper string) string
	timeout time.Duration
}

// newExecCredentialHelper returns the default, PATH-based credential helper runner.
func newExecCredentialHelper() *execCredentialHelper {
	return &execCredentialHelper{
		command: func(helper string) string { return credentialHelperPrefix + helper },
		timeout: credentialHelperTimeout,
	}
}

// Store saves the credentials through `docker-credential-<helper> store`.
func (h *execCredentialHelper) Store(helper, serverURL, username, secret string) error {
	defer perf.Track(nil, "docker.execCredentialHelper.Store")()

	payload, err := json.Marshal(helperCredentials{ServerURL: serverURL, Username: username, Secret: secret})
	if err != nil {
		return fmt.Errorf("%w: %s%s store for %s: %w", errUtils.ErrDockerCredentialHelper, credentialHelperPrefix, helper, serverURL, err)
	}
	if _, err := h.run(helper, helperActionStore, payload); err != nil {
		return fmt.Errorf("%w: %s%s store for %s: %w", errUtils.ErrDockerCredentialHelper, credentialHelperPrefix, helper, serverURL, err)
	}
	return nil
}

// Erase removes the credentials through `docker-credential-<helper> erase`.
func (h *execCredentialHelper) Erase(helper, serverURL string) error {
	defer perf.Track(nil, "docker.execCredentialHelper.Erase")()

	out, err := h.run(helper, helperActionErase, []byte(serverURL))
	if err != nil {
		// Logging out of a registry the helper does not know about is a no-op.
		if strings.TrimSpace(out) == helperNotFoundMessage {
			return nil
		}
		return fmt.Errorf("%w: %s%s erase for %s: %w", errUtils.ErrDockerCredentialHelper, credentialHelperPrefix, helper, serverURL, err)
	}
	return nil
}

// run executes the helper with a timeout and returns its stdout. Helpers report
// errors on stdout, so a non-empty stdout or stderr is included in the error
// rather than printed to the terminal.
func (h *execCredentialHelper) run(helper, action string, input []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, h.command(helper), action)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("%w: %w", err, ctx.Err())
	}
	if msg := strings.TrimSpace(stdout.String() + " " + stderr.String()); msg != "" {
		err = fmt.Errorf("%w: %s", err, msg)
	}
	return stdout.String(), err
}

// Compile-time check that the default runner implements CredentialHelper.
var _ CredentialHelper = (*execCredentialHelper)(nil)
