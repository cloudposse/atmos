package main

import (
	"regexp"
	"strings"
)

// commandSlug converts a manifest command into its artifact path, matching the
// slugs produced by the legacy bash pipeline so existing docs references keep
// working (e.g. "atmos about --help" → "atmos-about--help").
func commandSlug(command string) string {
	slug := strings.ReplaceAll(command, " --charset=UTF-8", "")
	slug = strings.ReplaceAll(slug, " -", "-")
	slug = regexp.MustCompile(`\s+`).ReplaceAllString(slug, "-")
	return strings.ReplaceAll(slug, "---", "--")
}

// noiseLinePatterns match whole lines of environment-dependent Terraform
// noise that must not appear in committed docs artifacts.
var noiseLinePatterns = []*regexp.Regexp{
	regexp.MustCompile(`- Finding latest version of`),
	regexp.MustCompile(`- Installed hashicorp`),
	regexp.MustCompile(`- Installing hashicorp`),
	regexp.MustCompile(`Terraform has created a lock file`),
	regexp.MustCompile(`Include this file in your version control repository`),
	regexp.MustCompile(`guarantee to make the same selections by default when`),
	regexp.MustCompile(`you run "terraform init" in the future`),
	regexp.MustCompile(`Workspace .* doesn.t exist.`),
	regexp.MustCompile(`You can create this workspace with the .* subcommand`),
	regexp.MustCompile(`or include the .* flag with the .* subcommand.`),
}

// noiseSubstitutions rewrite unstable fragments (heredoc markers, random
// resource IDs) inside surviving lines.
var noiseSubstitutions = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`Resource actions are indicated with the following symbols.*`), ""},
	{regexp.MustCompile(`(?m)^ *EOT`), "\n"},
	{regexp.MustCompile(` *<<-?EOT`), "\n"},
	{regexp.MustCompile(`\[id=[a-f0-9]+\]`), ""},
	{regexp.MustCompile(`(\[id=http)`), "\n    $1"},
}

// filterNoise ports the legacy pipeline's sed-based content scrubbing: it
// drops noisy Terraform bootstrap lines and normalizes unstable output so
// regenerated artifacts stay deterministic.
func filterNoise(content string) string {
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if matchesNoiseLine(line) {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "\n")
	for _, sub := range noiseSubstitutions {
		result = sub.pattern.ReplaceAllString(result, sub.replacement)
	}
	return result
}

func matchesNoiseLine(line string) bool {
	for _, pattern := range noiseLinePatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}
