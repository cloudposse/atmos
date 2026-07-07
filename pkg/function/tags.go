package function

import fntag "github.com/cloudposse/atmos/pkg/function/tag"

// Tag constants for Atmos configuration functions.
// These are compatibility aliases; pkg/function/tag is the central tag catalog.
const (
	// TagExec executes a shell command and returns the output.
	TagExec = fntag.Exec

	// TagSecret resolves a declared secret from its configured backend.
	TagSecret = fntag.Secret

	// TagStore retrieves a value from a configured store.
	TagStore = fntag.Store

	// TagStoreGet retrieves a value from a configured store (alternative syntax).
	TagStoreGet = fntag.StoreGet

	// TagTemplate processes a JSON template.
	TagTemplate = fntag.Template

	// TagTerraformOutput retrieves a Terraform output value.
	TagTerraformOutput = fntag.TerraformOutput

	// TagTerraformState retrieves a value from Terraform state.
	TagTerraformState = fntag.TerraformState

	// TagEnv retrieves an environment variable value.
	TagEnv = fntag.Env

	// TagCEL evaluates a Common Expression Language condition.
	TagCEL = fntag.CEL

	// TagInclude includes content from another file.
	TagInclude = fntag.Include

	// TagIncludeRaw includes raw content from another file.
	TagIncludeRaw = fntag.IncludeRaw

	// TagRepoRoot returns the git repository root path.
	TagRepoRoot = fntag.RepoRoot

	// TagGitRoot returns the git repository root path.
	TagGitRoot = fntag.GitRoot

	// TagGitSha returns the current Git HEAD commit SHA.
	TagGitSha = fntag.GitSha

	// TagGitBranch returns the current Git branch name.
	TagGitBranch = fntag.GitBranch

	// TagGitRef returns the immutable Git ref used for source pinning.
	TagGitRef = fntag.GitRef

	// TagGitRepository returns the repository owner/name slug.
	TagGitRepository = fntag.GitRepository

	// TagGitOwner returns the repository owner.
	TagGitOwner = fntag.GitOwner

	// TagGitName returns the repository name.
	TagGitName = fntag.GitName

	// TagGitHost returns the repository host.
	TagGitHost = fntag.GitHost

	// TagGitURL returns the repository URL.
	TagGitURL = fntag.GitURL

	// TagAppend appends list items during stack merging.
	TagAppend = fntag.Append

	// TagCwd returns the current working directory.
	TagCwd = fntag.Cwd

	// TagUnset removes a value during configuration processing.
	TagUnset = fntag.Unset

	// TagRandom generates a random number.
	TagRandom = fntag.Random

	// TagLiteral preserves values exactly as written, bypassing template processing.
	TagLiteral = fntag.Literal

	// TagAwsAccountID returns the AWS account ID.
	TagAwsAccountID = fntag.AwsAccountID

	// TagAwsCallerIdentityArn returns the AWS caller identity ARN.
	TagAwsCallerIdentityArn = fntag.AwsCallerIdentityArn

	// TagAwsCallerIdentityUserID returns the AWS caller identity user ID.
	TagAwsCallerIdentityUserID = fntag.AwsCallerIdentityUserID

	// TagAwsRegion returns the AWS region.
	TagAwsRegion = fntag.AwsRegion

	// TagAwsOrganizationID returns the AWS Organization ID.
	TagAwsOrganizationID = fntag.AwsOrganizationID

	// TagEmulator resolves a value from a local emulator.
	TagEmulator = fntag.Emulator

	// TagVersion resolves a locked version from the Atmos Version Tracker.
	TagVersion = fntag.Version
)

// YAMLTagPrefix is the prefix used for YAML custom tags.
const YAMLTagPrefix = fntag.YAMLPrefix

// AllTags returns all registered tag names.
func AllTags() []string {
	return fntag.All()
}

// IsValidTag checks if the given tag name is registered.
func IsValidTag(tag string) bool {
	return fntag.IsValid(tag)
}

// YAMLTag returns the YAML tag format for a function name (e.g., "env" -> "!env").
func YAMLTag(name string) string {
	return fntag.ToYAML(name)
}

// FromYAMLTag extracts the function name from a YAML tag (e.g., "!env" -> "env").
func FromYAMLTag(tag string) string {
	return fntag.FromYAML(tag)
}

// AllYAMLTags returns all registered tag names with the YAML prefix (e.g., "!env", "!exec").
// This is useful for error messages that need to show supported YAML tags.
func AllYAMLTags() []string {
	return fntag.AllYAML()
}

// IsValidYAMLTag checks if a YAML tag is registered. The tag should include the YAML prefix.
func IsValidYAMLTag(tag string) bool {
	return fntag.IsValidYAML(tag)
}
