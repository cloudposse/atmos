package tag

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// MatchesTag reports whether input starts with prefix as a complete YAML tag.
// Any character that could extend the tag name (letters, digits, '.', '-', '_')
// disqualifies the match; anything else (whitespace, but also punctuation like
// '[', '{', '"' that unambiguously starts a value) is a valid boundary. The
// whitespace-only check this replaced broke values whose Go template rendering
// trims the separator (e.g. a `{{-` marker), producing `!template[...]` with no
// space before the content.
//
// The prefix includes the YAML prefix, for example "!terraform.state".
func MatchesTag(input, prefix string) bool {
	defer perf.Track(nil, "tag.MatchesTag")()

	if !strings.HasPrefix(input, prefix) {
		return false
	}
	rest := strings.TrimPrefix(input, prefix)
	if rest == "" {
		return true
	}
	return !continuesTagName(rest[0])
}

// continuesTagName reports whether c could extend a tag name.
func continuesTagName(c byte) bool {
	return c == '.' || c == '-' || c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// stringFormTags are the functions the legacy stack YAML processor resolves when they appear
// at the start of a string value, including a quoted value that carries no YAML tag.
// These are the tags handled by processSimpleTags and processContextAwareTags in
// internal/exec/yaml_func_utils.go. The remaining registered functions (!cel, !include,
// !include.raw, !append, !literal, !starlark) are consumed from the YAML
// node tag while loading, so a quoted occurrence is plain text.
var stringFormTags = []string{
	Exec, Secret, Store, StoreGet, Template, TerraformOutput, TerraformState, Env, Random, Cwd,
	RepoRoot, GitRoot, GitSha, GitBranch, GitRef, GitRepository, GitOwner, GitName, GitHost, GitURL,
	Unset, AwsAccountID, AwsCallerIdentityArn, AwsCallerIdentityUserID, AwsRegion, AwsOrganizationID,
	AwsCloudFormationOutput, CILabels, Emulator, Version, Tags, Labels, LabelsKeys, LabelsValues,
}

// StringFormYAML returns the YAML tags (with the "!" prefix) that legacy editions resolve when
// they start an untagged string value.
func StringFormYAML() []string {
	defer perf.Track(nil, "tag.StringFormYAML")()

	result := make([]string, len(stringFormTags))
	for i, name := range stringFormTags {
		result[i] = ToYAML(name)
	}
	return result
}

// MatchesStringForm reports whether legacy editions treat the untagged string value as a
// call to a string-form function, applying the same tag boundary rule as the runtime.
func MatchesStringForm(value string) bool {
	defer perf.Track(nil, "tag.MatchesStringForm")()

	if !strings.HasPrefix(value, YAMLPrefix) {
		return false
	}
	for _, name := range stringFormTags {
		if MatchesTag(value, ToYAML(name)) {
			return true
		}
	}
	return false
}

// IsSkipped matches a function name against bare or YAML-prefixed skip entries.
func IsSkipped(skip []string, name string) bool {
	defer perf.Track(nil, "tag.IsSkipped")()
	name = strings.TrimPrefix(name, "!")
	for _, entry := range skip {
		if strings.TrimPrefix(entry, "!") == name {
			return true
		}
	}
	return false
}
