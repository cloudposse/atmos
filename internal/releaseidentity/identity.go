// Package releaseidentity defines the identity embedded in release binaries.
package releaseidentity

import (
	"errors"
	"fmt"
	"regexp"
)

// Value is set by the release linker. Version and Commit are derived from this
// same record, so verification does not depend on symbols removed by -s or
// linker flags omitted from Go build information by -trimpath.
var Value string

// ErrInvalidIdentity indicates a malformed release identity.
var ErrInvalidIdentity = errors.New("invalid release identity")

// Pattern matches a complete embedded release identity, including prereleases.
var Pattern = regexp.MustCompile(`atmos-release:(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?):([0-9a-f]{40}):end`)

// Parse returns the CLI version (without v) and full source commit.
//
//nolint:lintroller // Called during version package initialization; keep identity parsing independent of application instrumentation.
func Parse(value string) (string, string, error) {
	match := Pattern.FindStringSubmatch(value)
	if len(match) != 3 || match[0] != value {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidIdentity, value)
	}
	return match[1][1:], match[2], nil
}
