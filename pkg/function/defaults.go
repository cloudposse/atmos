package function

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultRegistry creates a new registry with all default functions registered.
// The shellExecutor parameter provides the implementation for the !exec function.
// If shellExecutor is nil, the !exec function will not be available.
func DefaultRegistry(shellExecutor ShellExecutor) *Registry {
	defer perf.Track(nil, "function.DefaultRegistry")()

	r := NewRegistry()

	// Register pre-merge functions.
	_ = r.Register(NewEnvFunction())
	_ = r.Register(NewTemplateFunction())
	_ = r.Register(NewGitRootFunction())
	_ = r.Register(NewIncludeFunction())
	_ = r.Register(NewIncludeRawFunction())
	_ = r.Register(NewRandomFunction())

	// Register exec function only if executor is provided.
	if shellExecutor != nil {
		_ = r.Register(NewExecFunction(shellExecutor))
	}

	// Register post-merge placeholder functions.
	// These return placeholders for later resolution when stack context is available.
	_ = r.Register(NewTerraformOutputFunction())
	_ = r.Register(NewTerraformStateFunction())
	_ = r.Register(NewStoreFunction())
	_ = r.Register(NewStoreGetFunction())
	_ = r.Register(NewAwsAccountIDFunction())
	_ = r.Register(NewAwsCallerIdentityArnFunction())
	_ = r.Register(NewAwsCallerIdentityUserIDFunction())
	_ = r.Register(NewAwsRegionFunction())

	return r
}

// Tags returns a map of function tags to function names.
// This is useful for detecting function calls in string values.
func Tags() map[string]string {
	defer perf.Track(nil, "function.Tags")()

	return map[string]string{
		// Pre-merge functions.
		TagEnv:        "env",
		TagExec:       "exec",
		TagTemplate:   "template",
		TagRepoRoot:   "repo-root",
		TagInclude:    "include",
		TagIncludeRaw: "include.raw",
		TagRandom:     "random",
		// Post-merge functions.
		TagTerraformOutput:         "terraform.output",
		TagTerraformState:          "terraform.state",
		TagStore:                   "store",
		TagStoreGet:                "store.get",
		TagAwsAccountID:            "aws.account_id",
		TagAwsCallerIdentityArn:    "aws.caller_identity_arn",
		TagAwsCallerIdentityUserID: "aws.caller_identity_user_id",
		TagAwsRegion:               "aws.region",
	}
}

// AllTags is defined in tags.go for centralized tag management.
