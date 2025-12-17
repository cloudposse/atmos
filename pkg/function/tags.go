package function

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// Tag constants for Atmos configuration functions.
// These are the canonical function names used across all formats.
// In YAML they appear as !tag, in HCL as tag(), etc.
const (
	// TagExec executes a shell command and returns the output.
	TagExec = "exec"

	// TagStore retrieves a value from a configured store.
	TagStore = "store"

	// TagStoreGet retrieves a value from a configured store (alternative syntax).
	TagStoreGet = "store.get"

	// TagTemplate processes a JSON template.
	TagTemplate = "template"

	// TagTerraformOutput retrieves a Terraform output value.
	TagTerraformOutput = "terraform.output"

	// TagTerraformState retrieves a value from Terraform state.
	TagTerraformState = "terraform.state"

	// TagEnv retrieves an environment variable value.
	TagEnv = "env"

	// TagInclude includes content from another file.
	TagInclude = "include"

	// TagIncludeRaw includes raw content from another file.
	TagIncludeRaw = "include.raw"

	// TagRepoRoot returns the git repository root path.
	TagRepoRoot = "repo-root"

	// TagRandom generates a random number.
	TagRandom = "random"

	// TagLiteral preserves values exactly as written, bypassing template processing.
	TagLiteral = "literal"

	// TagAwsAccountID returns the AWS account ID.
	TagAwsAccountID = "aws.account_id"

	// TagAwsCallerIdentityArn returns the AWS caller identity ARN.
	TagAwsCallerIdentityArn = "aws.caller_identity_arn"

	// TagAwsCallerIdentityUserID returns the AWS caller identity user ID.
	TagAwsCallerIdentityUserID = "aws.caller_identity_user_id"

	// TagAwsRegion returns the AWS region.
	TagAwsRegion = "aws.region"
)

// YAMLTagPrefix is the prefix used for YAML custom tags.
const YAMLTagPrefix = "!"

// AllTags returns all registered tag names.
func AllTags() []string {
	defer perf.Track(nil, "function.AllTags")()

	return []string{
		TagExec,
		TagStore,
		TagStoreGet,
		TagTemplate,
		TagTerraformOutput,
		TagTerraformState,
		TagEnv,
		TagInclude,
		TagIncludeRaw,
		TagRepoRoot,
		TagRandom,
		TagLiteral,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
	}
}

// TagsMap provides O(1) lookup for tag names.
var TagsMap = map[string]bool{
	TagExec:                    true,
	TagStore:                   true,
	TagStoreGet:                true,
	TagTemplate:                true,
	TagTerraformOutput:         true,
	TagTerraformState:          true,
	TagEnv:                     true,
	TagInclude:                 true,
	TagIncludeRaw:              true,
	TagRepoRoot:                true,
	TagRandom:                  true,
	TagLiteral:                 true,
	TagAwsAccountID:            true,
	TagAwsCallerIdentityArn:    true,
	TagAwsCallerIdentityUserID: true,
	TagAwsRegion:               true,
}

// YAMLTag returns the YAML tag format for a function name (e.g., "env" -> "!env").
func YAMLTag(name string) string {
	defer perf.Track(nil, "function.YAMLTag")()

	return YAMLTagPrefix + name
}

// FromYAMLTag extracts the function name from a YAML tag (e.g., "!env" -> "env").
func FromYAMLTag(tag string) string {
	defer perf.Track(nil, "function.FromYAMLTag")()

	if len(tag) > 0 && tag[0] == '!' {
		return tag[1:]
	}
	return tag
}
