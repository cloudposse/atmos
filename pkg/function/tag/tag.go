// Package tag provides YAML tag constants and validation for Atmos functions.
// This package is intentionally minimal and dependency-free to avoid import cycles
// when used from pkg/utils.
package tag

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// Tag constants for Atmos configuration functions.
// These are the canonical function names used across all formats.
// In YAML they appear as !tag, in HCL as tag(), etc.
const (
	// Exec executes a shell command and returns the output.
	Exec = "exec"

	// Store retrieves a value from a configured store.
	Store = "store"

	// StoreGet retrieves a value from a configured store (alternative syntax).
	StoreGet = "store.get"

	// Template processes a JSON template.
	Template = "template"

	// TerraformOutput retrieves a Terraform output value.
	TerraformOutput = "terraform.output"

	// TerraformState retrieves a value from Terraform state.
	TerraformState = "terraform.state"

	// Env retrieves an environment variable value.
	Env = "env"

	// Include includes content from another file.
	Include = "include"

	// IncludeRaw includes raw content from another file.
	IncludeRaw = "include.raw"

	// RepoRoot returns the git repository root path.
	RepoRoot = "repo-root"

	// Cwd returns the current working directory.
	Cwd = "cwd"

	// Random generates a random number.
	Random = "random"

	// Literal preserves values exactly as written, bypassing template processing.
	Literal = "literal"

	// AwsAccountID returns the AWS account ID.
	AwsAccountID = "aws.account_id"

	// AwsCallerIdentityArn returns the AWS caller identity ARN.
	AwsCallerIdentityArn = "aws.caller_identity_arn"

	// AwsCallerIdentityUserID returns the AWS caller identity user ID.
	AwsCallerIdentityUserID = "aws.caller_identity_user_id"

	// AwsRegion returns the AWS region.
	AwsRegion = "aws.region"
)

// YAMLPrefix is the prefix used for YAML custom tags.
const YAMLPrefix = "!"

// All returns all registered tag names without the YAML prefix.
func All() []string {
	defer perf.Track(nil, "tag.All")()

	return []string{
		Exec,
		Store,
		StoreGet,
		Template,
		TerraformOutput,
		TerraformState,
		Env,
		Include,
		IncludeRaw,
		RepoRoot,
		Cwd,
		Random,
		Literal,
		AwsAccountID,
		AwsCallerIdentityArn,
		AwsCallerIdentityUserID,
		AwsRegion,
	}
}

// tagsMap provides O(1) lookup for tag names.
var tagsMap = map[string]bool{
	Exec:                    true,
	Store:                   true,
	StoreGet:                true,
	Template:                true,
	TerraformOutput:         true,
	TerraformState:          true,
	Env:                     true,
	Include:                 true,
	IncludeRaw:              true,
	RepoRoot:                true,
	Cwd:                     true,
	Random:                  true,
	Literal:                 true,
	AwsAccountID:            true,
	AwsCallerIdentityArn:    true,
	AwsCallerIdentityUserID: true,
	AwsRegion:               true,
}

// IsValid checks if a tag name is registered. The name should not include the YAML prefix.
func IsValid(name string) bool {
	defer perf.Track(nil, "tag.IsValid")()

	return tagsMap[name]
}

// ToYAML returns the YAML tag format for a function name (e.g., "env" -> "!env").
func ToYAML(name string) string {
	defer perf.Track(nil, "tag.ToYAML")()

	return YAMLPrefix + name
}

// FromYAML extracts the function name from a YAML tag (e.g., "!env" -> "env").
func FromYAML(yamlTag string) string {
	defer perf.Track(nil, "tag.FromYAML")()

	if len(yamlTag) > 0 && yamlTag[0] == '!' {
		return yamlTag[1:]
	}
	return yamlTag
}

// AllYAML returns all registered tag names with the YAML prefix (e.g., "!env", "!exec").
// This is useful for error messages that need to show supported YAML tags.
func AllYAML() []string {
	defer perf.Track(nil, "tag.AllYAML")()

	tags := All()
	result := make([]string, len(tags))
	for i, t := range tags {
		result[i] = ToYAML(t)
	}
	return result
}

// IsValidYAML checks if a YAML tag is registered. The tag should include the YAML prefix.
func IsValidYAML(yamlTag string) bool {
	defer perf.Track(nil, "tag.IsValidYAML")()

	return IsValid(FromYAML(yamlTag))
}
