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

	// Secret resolves a declared secret from its configured backend.
	Secret = "secret"

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

	// GitRoot returns the git repository root path.
	GitRoot = "git.root"

	// GitSha returns the current Git HEAD commit SHA.
	GitSha = "git.sha"

	// GitBranch returns the current Git branch name.
	GitBranch = "git.branch"

	// GitRef returns the immutable Git ref used for source pinning.
	GitRef = "git.ref"

	// GitRepository returns the repository owner/name slug.
	GitRepository = "git.repository"

	// GitOwner returns the repository owner.
	GitOwner = "git.owner"

	// GitName returns the repository name.
	GitName = "git.name"

	// GitHost returns the repository host.
	GitHost = "git.host"

	// GitURL returns the repository URL.
	GitURL = "git.url"

	// Append appends list items during stack merging.
	Append = "append"

	// Cwd returns the current working directory.
	Cwd = "cwd"

	// Unset removes a value during configuration processing.
	Unset = "unset"

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

	// AwsOrganizationID returns the AWS Organization ID.
	AwsOrganizationID = "aws.organization_id"

	// Emulator resolves a value from a local emulator.
	Emulator = "emulator"
)

// YAMLPrefix is the prefix used for YAML custom tags.
const YAMLPrefix = "!"

// All returns all registered tag names without the YAML prefix.
func All() []string {
	defer perf.Track(nil, "tag.All")()

	return []string{
		Exec,
		Secret,
		Store,
		StoreGet,
		Template,
		TerraformOutput,
		TerraformState,
		Env,
		Include,
		IncludeRaw,
		RepoRoot,
		GitRoot,
		GitSha,
		GitBranch,
		GitRef,
		GitRepository,
		GitOwner,
		GitName,
		GitHost,
		GitURL,
		Append,
		Cwd,
		Unset,
		Random,
		Literal,
		AwsAccountID,
		AwsCallerIdentityArn,
		AwsCallerIdentityUserID,
		AwsRegion,
		AwsOrganizationID,
		Emulator,
	}
}

// tagsMap provides O(1) lookup for tag names.
var tagsMap = map[string]bool{
	Exec:                    true,
	Secret:                  true,
	Store:                   true,
	StoreGet:                true,
	Template:                true,
	TerraformOutput:         true,
	TerraformState:          true,
	Env:                     true,
	Include:                 true,
	IncludeRaw:              true,
	RepoRoot:                true,
	GitRoot:                 true,
	GitSha:                  true,
	GitBranch:               true,
	GitRef:                  true,
	GitRepository:           true,
	GitOwner:                true,
	GitName:                 true,
	GitHost:                 true,
	GitURL:                  true,
	Append:                  true,
	Cwd:                     true,
	Unset:                   true,
	Random:                  true,
	Literal:                 true,
	AwsAccountID:            true,
	AwsCallerIdentityArn:    true,
	AwsCallerIdentityUserID: true,
	AwsRegion:               true,
	AwsOrganizationID:       true,
	Emulator:                true,
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

// AtmosConfigYAML returns the YAML tags supported while preprocessing atmos.yaml.
// Some registered YAML functions require stack/component context and are resolved
// later by the stack YAML processor, so they are intentionally excluded here.
func AtmosConfigYAML() []string {
	defer perf.Track(nil, "tag.AtmosConfigYAML")()

	return []string{
		ToYAML(Env),
		ToYAML(Exec),
		ToYAML(Include),
		ToYAML(IncludeRaw),
		ToYAML(RepoRoot),
		ToYAML(GitRoot),
		ToYAML(GitSha),
		ToYAML(GitBranch),
		ToYAML(GitRef),
		ToYAML(GitRepository),
		ToYAML(GitOwner),
		ToYAML(GitName),
		ToYAML(GitHost),
		ToYAML(GitURL),
		ToYAML(Cwd),
		ToYAML(Random),
		ToYAML(Unset),
	}
}

// IsAtmosConfigYAML checks if a YAML tag is supported by atmos.yaml preprocessing.
func IsAtmosConfigYAML(yamlTag string) bool {
	defer perf.Track(nil, "tag.IsAtmosConfigYAML")()

	for _, tag := range AtmosConfigYAML() {
		if yamlTag == tag {
			return true
		}
	}
	return false
}
