package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The root command enables Viper's AutomaticEnv() with the ATMOS prefix, and Viper resolves
// ATMOS_<KEY> before explicitly bound environment variables. A command that reads the bare Viper
// keys "tags" or "labels" therefore silently picks up a job-level ATMOS_TAGS / ATMOS_LABELS, which
// belong to the terraform family. Every non-terraform command must namespace those keys with
// flags.WithViperKey (for example "list.tags").

var (
	// A Viper-style read of a bare selector key. Cobra reads (cmd.Flags().GetString("tags")) are
	// excluded by selectorReadAllowedReceiver.
	bareSelectorReadPattern = regexp.MustCompile(`(\S+)\.(?:GetString|GetStringSlice|IsSet)\("(?:tags|labels)"\)`)
	// A selector flag definition (literal names or the shared constants).
	selectorFlagDefinitionPattern = regexp.MustCompile(`With(?:String|StringSlice)Flag\((?:"(?:tags|labels)"|(?:flagTags|flagLabels|tagsFlagName)\b)`)
	viperKeyOptionPattern         = regexp.MustCompile(`\bWithViperKey\(`)
)

// selectorGuardExemptDirs are top-level cmd directories excluded from the guard. Terraform binds
// ATMOS_TAGS / ATMOS_LABELS on purpose (cmd/terraform/flags.go) and shares them with describe affected.
var selectorGuardExemptDirs = map[string]bool{"terraform": true}

// selectorGuardExemptFiles are files (slash-separated, relative to cmd/) exempt from the guard,
// each with the reason it is safe.
var selectorGuardExemptFiles = map[string]string{
	// Uses its own "describe.affected" Viper prefix for --tags/--labels.
	"describe_affected_selector_flags.go": "namespaced with the describe.affected Viper prefix",
	// These commands define --tags/--labels but read them straight from their Cobra flags, never
	// from Viper, so ATMOS_TAGS / ATMOS_LABELS cannot reach them.
	"vendor/clean.go":          "reads --tags/--labels from Cobra flags only",
	"container/verbs.go":       "reads --tags/--labels from Cobra flags only",
	"workflow/workflow.go":     "reads --tags/--labels from Cobra flags only",
	"helm/helm.go":             "reads --tags/--labels from Cobra flags only",
	"kubernetes/kubernetes.go": "reads --tags/--labels from Cobra flags only",
}

// selectorReadAllowedReceiver reports whether a receiver expression is a Cobra or pflag flag set, from which reading the bare tags and labels keys is allowed.
func selectorReadAllowedReceiver(receiver string) bool {
	return strings.Contains(receiver, "Flags()") || strings.HasSuffix(receiver, "flags") || strings.HasSuffix(receiver, "flagSet")
}

// selectorGuardViolations scans non-test Go source under root and returns one message per violation.
// It also returns how many files were scanned so callers can fail loudly on a misconfigured root.
func selectorGuardViolations(t *testing.T, root string) ([]string, int) {
	t.Helper()

	var violations []string
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if selectorGuardExemptDirs[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		scanned++
		if _, exempt := selectorGuardExemptFiles[rel]; exempt {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		violations = append(violations, scanSelectorSource(rel, string(src))...)
		return nil
	})
	require.NoError(t, err)

	return violations, scanned
}

// scanSelectorSource applies the guard rules to one file's source.
func scanSelectorSource(rel, src string) []string {
	var violations []string
	definitions := 0

	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		for _, m := range bareSelectorReadPattern.FindAllStringSubmatch(line, -1) {
			if !selectorReadAllowedReceiver(m[1]) {
				violations = append(violations, rel+":"+strconv.Itoa(i+1)+": reads a bare Viper selector key ("+strings.TrimSpace(m[0])+"); use a namespaced key set with flags.WithViperKey")
			}
		}
		if selectorFlagDefinitionPattern.MatchString(line) {
			definitions++
		}
	}

	if overrides := len(viperKeyOptionPattern.FindAllString(src, -1)); definitions > overrides {
		violations = append(violations, rel+": defines "+strconv.Itoa(definitions)+" --tags/--labels flag(s) but only "+strconv.Itoa(overrides)+" flags.WithViperKey override(s)")
	}
	return violations
}

// TestSelectorFlagsUseNamespacedViperKeys verifies that non-terraform commands do not bind or read the bare Viper keys tags and labels.
func TestSelectorFlagsUseNamespacedViperKeys(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to resolve the test file location")
	cmdDir := filepath.Dir(thisFile)

	violations, scanned := selectorGuardViolations(t, cmdDir)

	require.Positive(t, scanned, "the guard scanned no Go files under %s", cmdDir)
	assert.Empty(t, violations, "non-terraform commands must not bind or read the bare Viper keys \"tags\"/\"labels\" "+
		"(ATMOS_TAGS / ATMOS_LABELS would leak in through AutomaticEnv); namespace them with flags.WithViperKey")
}

// TestSelectorGuard_DetectsViolations proves the guard's rules fire on the patterns it exists to catch
// and stay quiet on the compliant forms.
func TestSelectorGuard_DetectsViolations(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"bare viper read", "x := v.GetString(\"tags\")", 1},
		{"bare viper slice read", "x := viper.GetStringSlice(\"tags\")", 1},
		{"bare labels read", "x := v.GetString(\"labels\")", 1},
		{"cobra flag read is allowed", "x, _ := cmd.Flags().GetString(\"tags\")", 0},
		{"commented read is ignored", "// v.GetString(\"tags\")", 0},
		{"definition without override", "flags.WithStringFlag(\"tags\", \"\", \"\", \"Tags\"),", 1},
		{"slice definition without override", "flags.WithStringSliceFlag(\"labels\", \"\", nil, \"Labels\"),", 1},
		{"definition with override", "flags.WithStringFlag(\"tags\", \"\", \"\", \"Tags\"),\nflags.WithViperKey(\"tags\", \"x.tags\"),", 0},
		{"unrelated flag", "flags.WithStringFlag(\"stack\", \"s\", \"\", \"Stack\"),", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, scanSelectorSource("fixture.go", tt.src), tt.want)
		})
	}
}
