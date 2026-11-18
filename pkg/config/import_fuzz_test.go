package config

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/artifact/redacttest"
)

// TestSanitizeImport_GeneratedInputs runs the generated inputs, built from every delimiter of the
// shared table, through SanitizeImport.
func TestSanitizeImport_GeneratedInputs(t *testing.T) {
	for _, separator := range redacttest.Seeds() {
		for _, in := range redacttest.Generate(redacttest.SecretOf("A", separator, "B")) {
			redacttest.RequireNoLeak(t, "SanitizeImport", in, SanitizeImport(in.Raw))
		}
	}
}

// FuzzSanitizeImport embeds a random secret in many address templates and checks that
// SanitizeImport never leaks the user name or any fragment of the secret.
func FuzzSanitizeImport(f *testing.F) {
	for _, separator := range redacttest.Seeds() {
		f.Add("A", separator, "B")
		f.Add("", separator, "")
	}
	f.Fuzz(func(t *testing.T, first, separator, second string) {
		for _, in := range redacttest.Generate(redacttest.SecretOf(first, separator, second)) {
			redacttest.RequireNoLeak(t, "SanitizeImport", in, SanitizeImport(in.Raw))
		}
	})
}
