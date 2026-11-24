package docker

import (
	"errors"
	"fmt"
)

// helperManagedError is returned by WriteAuth when config.json routes the
// registry to a credential helper that does not implement store, such as
// docker-credential-ecr-login. Such a helper gets registry credentials itself,
// and Docker uses it instead of anything Atmos could write, so config.json is
// left unchanged. It wraps ErrDockerCredentialHelperStoreUnsupported.
type helperManagedError struct {
	registry string
	helper   string
	err      error
}

// Error returns the underlying helper failure.
func (e *helperManagedError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying helper failure.
func (e *helperManagedError) Unwrap() error {
	return e.err
}

// HelperManagedNotice reports whether err, returned by WriteAuth, means the
// registry is managed by a credential helper that gets credentials itself. It
// then returns the message to show instead of failing the login. Any other
// error, including a helper whose store genuinely fails, returns "" and false.
func HelperManagedNotice(err error) (string, bool) {
	var managed *helperManagedError
	if !errors.As(err, &managed) {
		return "", false
	}
	return fmt.Sprintf("Docker uses %s%s for %s, which gets registry credentials itself, so Atmos did not write any to %s",
		credentialHelperPrefix, managed.helper, managed.registry, configFileName), true
}

// selfResolvingHelpers are credential helpers that get registry credentials themselves instead
// of returning stored ones. docker-credential-ecr-login answers `get` by calling ECR with the AWS
// credentials in its own environment, and replies "not implemented" to `store`.
var selfResolvingHelpers = map[string]struct{}{
	"ecr-login": {},
}

// helperResolvesCredentialsItself reports whether helper gets registry credentials itself, so
// Atmos must not run its `get`: that would mint credentials with the ambient cloud identity just
// to take a snapshot. The `store` still runs, and its "not implemented" reply marks the registry
// as helper-managed (see HelperManagedNotice).
func helperResolvesCredentialsItself(helper string) bool {
	_, ok := selfResolvingHelpers[helper]
	return ok
}
