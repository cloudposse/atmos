package utils

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/artifact/redacttest"
)

var errIncludeFetch = errors.New("include fetch failed")

// checkIncludeEntryPoints runs one generated input through every include redaction entry point.
func checkIncludeEntryPoints(t testing.TB, in redacttest.Input) {
	t.Helper()
	redacttest.RequireNoLeak(t, "RedactIncludeArgument", in, RedactIncludeArgument(in.Raw))
	for _, message := range in.Messages() {
		err := redactIncludeError(fmt.Errorf("%w: %s", errIncludeFetch, message), in.Raw)
		redacttest.RequireNoLeak(t, "redactIncludeError", in, err.Error())
		if !errors.Is(err, errIncludeFetch) {
			t.Fatalf("redactIncludeError dropped the cause of %q", message)
		}
	}
}

// TestRedactInclude_SharedCredentialCases runs the shared table of hostile inputs through the
// include entry points. A scheme-less input is not URL-shaped, so only its error text is checked.
func TestRedactInclude_SharedCredentialCases(t *testing.T) {
	for _, tc := range redacttest.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			got := RedactIncludeArgument(tc.Raw)
			if strings.Contains(tc.Raw, "://") {
				for _, secret := range tc.Secrets {
					assert.NotContains(t, got, secret, "RedactIncludeArgument of %q", tc.Raw)
				}
			}
			for _, message := range []string{"fetch " + tc.Raw + " failed", fmt.Sprintf("fetch %q failed", tc.Raw)} {
				err := redactIncludeError(fmt.Errorf("%w: %s", errIncludeFetch, message), tc.Raw)
				for _, secret := range tc.Secrets {
					assert.NotContains(t, err.Error(), secret, "redactIncludeError of %q", message)
				}
			}
		})
	}
}

// FuzzRedactInclude embeds a random secret in many address templates and checks that no include
// entry point leaks the user name or any fragment of the secret.
func FuzzRedactInclude(f *testing.F) {
	for _, separator := range redacttest.Seeds() {
		f.Add("A", separator, "B")
		f.Add("", separator, "")
	}
	f.Fuzz(func(t *testing.T, first, separator, second string) {
		for _, in := range redacttest.Generate(redacttest.SecretOf(first, separator, second)) {
			// An argument that is not URL-shaped is a local path and keeps its text by design.
			if !strings.Contains(in.Raw, "://") {
				for _, message := range in.Messages() {
					redacttest.RequireNoLeak(t, "redactIncludeError", in, redactIncludeError(fmt.Errorf("%w: %s", errIncludeFetch, message), in.Raw).Error())
				}
				continue
			}
			checkIncludeEntryPoints(t, in)
		}
	})
}
