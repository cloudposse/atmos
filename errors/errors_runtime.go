package errors

import (
	"errors"
	"fmt"
)

// ErrStarlark identifies embedded Starlark execution or configuration failures.
var ErrStarlark = errors.New("starlark execution failed")

// ErrScriptRaised identifies an intentional failure raised by a script.
var ErrScriptRaised = errors.New("script raised a failure")

var (
	// ErrStarlarkInvalidArgument identifies invalid arguments passed to an embedded Starlark builtin.
	ErrStarlarkInvalidArgument = errors.New("invalid starlark argument")
	// ErrStarlarkTaskTimeout identifies a Starlark task that exceeded its deadline.
	ErrStarlarkTaskTimeout = errors.New("starlark task timed out")
	// ErrStarlarkProcessFailed identifies a subprocess started by Starlark that failed or never started.
	ErrStarlarkProcessFailed = errors.New("starlark process failed")
	// ErrStarlarkOutputEncode identifies a top-level Starlark `output` value that cannot be encoded.
	ErrStarlarkOutputEncode = errors.New("starlark output encoding failed")
	// ErrStarlarkRecursionLimit identifies a Starlark call stack that exceeded the maximum recursion depth.
	ErrStarlarkRecursionLimit = errors.New("starlark recursion depth exceeded")
	// ErrStarlarkStackIdentity identifies a stack name derived from a !starlark value, which is not computable.
	ErrStarlarkStackIdentity = errors.New("stack identity cannot be computed with !starlark")
	// ErrScriptUsage identifies command-line input that does not satisfy a standalone script's declared interface.
	ErrScriptUsage = errors.New("usage error")
)

const (
	// ErrWrapFormat is the standard format string for wrapping errors with context.
	// Use with fmt.Errorf to wrap a sentinel error with an underlying error:
	//   fmt.Errorf(ErrWrapFormat, errUtils.ErrSentinel, underlyingErr)
	ErrWrapFormat = "%w: %w"
	// ErrWrapWithNameFormat is the format string for wrapping errors with a name context.
	// Use with fmt.Errorf to wrap a sentinel error with a name:
	//   fmt.Errorf(ErrWrapWithNameFormat, errUtils.ErrSentinel, name)
	ErrWrapWithNameFormat = "%w: %s"
	// ErrWrapWithNameAndCauseFormat is the format string for wrapping errors with a name and cause.
	// Use with fmt.Errorf to wrap a sentinel error with a name and underlying error:
	//   fmt.Errorf(ErrWrapWithNameAndCauseFormat, errUtils.ErrSentinel, name, underlyingErr)
	ErrWrapWithNameAndCauseFormat = "%w '%s': %w"
)

// Provision-target authentication errors.
var (
	// ErrProvisionTargetAuthInvalid indicates a provision target's auth block is malformed.
	ErrProvisionTargetAuthInvalid = errors.New("provision target auth is invalid")
	// ErrProvisionTargetAuthUnknownKey indicates a provision target's auth block has an unsupported key.
	ErrProvisionTargetAuthUnknownKey = errors.New("provision target auth has an unsupported key")
	// ErrProvisionTargetAuthNoIdentity indicates a provision target's auth block selects no identity.
	ErrProvisionTargetAuthNoIdentity = errors.New("provision target auth did not select an identity")
	// ErrProvisionTargetAuthFailed indicates the identity a provision target selected could not be used.
	ErrProvisionTargetAuthFailed = errors.New("provision target authentication failed")
	// ErrAwsCloudFormationTargetAuthFailed indicates a CloudFormation delivery target's own authentication failed.
	ErrAwsCloudFormationTargetAuthFailed = errors.New("aws/cloudformation provision target could not authenticate")
	// ErrAwsCloudFormationTargetKeyUnsupported indicates a `kind: aws/cloudformation` target has an unsupported key.
	ErrAwsCloudFormationTargetKeyUnsupported = errors.New("aws/cloudformation provision target has an unsupported key")
)

// Stack dependency (`depends_on`) resolution errors.
var (
	// ErrInvalidDependsOn indicates a legacy `settings.spacelift.depends_on` value
	// does not resolve to a known stack or to a component in the current stack.
	ErrInvalidDependsOn = errors.New("invalid 'depends_on' dependency")
	// ErrInvalidSettingsDependsOn indicates a `settings.depends_on` value does not
	// resolve to a known component in a known stack.
	ErrInvalidSettingsDependsOn = errors.New("invalid 'settings.depends_on' dependency")
)

// Vendor update/diff errors.
var (
	// ErrComponentUpdaterConfig indicates invalid component updater configuration.
	ErrComponentUpdaterConfig = errors.New("invalid component updater configuration")
	// ErrComponentUpdaterDirtyWorktree indicates a publisher's Git worktree has uncommitted changes.
	ErrComponentUpdaterDirtyWorktree = errors.New("component updater requires a clean Git worktree")
	// ErrPullRequestPublisherUnavailable indicates the selected forge integration is not registered.
	ErrPullRequestPublisherUnavailable = errors.New("pull request publisher is unavailable")
	// ErrGitHubAuthorization indicates an authentication or permission failure from the GitHub API.
	ErrGitHubAuthorization = errors.New("GitHub authorization failed")
	// ErrPullRequestReconciliation indicates Atmos could not reconcile an existing or new PR.
	ErrPullRequestReconciliation = errors.New("pull request reconciliation failed")
	// ErrAzureDevOpsTokenNotFound indicates AZURE_DEVOPS_EXT_PAT is not set.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsTokenNotFound = errors.New("Azure DevOps personal access token not found")
	// ErrAzureDevOpsAuthorization indicates an authentication or permission failure from the Azure DevOps API.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsAuthorization = errors.New("Azure DevOps authorization failed")
	// ErrAzureDevOpsAssigneesUnsupported indicates Azure DevOps pull requests do not support assignees.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsAssigneesUnsupported = errors.New("Azure DevOps pull requests do not support assignees")
	// ErrAzureDevOpsNamespaceInvalid indicates PullRequestOptions.Namespace was not exactly the
	// single project segment Azure DevOps' organization/project/repository addressing requires.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsNamespaceInvalid = errors.New("Azure DevOps pull request namespace must be exactly one project segment")
	// ErrAzureDevOpsReviewerNotFound indicates a configured reviewer's display name, account name,
	// or email matched no Azure DevOps identity.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsReviewerNotFound = errors.New("Azure DevOps reviewer identity not found")
	// ErrAzureDevOpsReviewerAmbiguous indicates a configured reviewer's display name, account name,
	// or email matched more than one Azure DevOps identity.
	//nolint:staticcheck // Error starts with a proper product name.
	ErrAzureDevOpsReviewerAmbiguous = errors.New("Azure DevOps reviewer identity is ambiguous")
	// ErrGitFetchFailed indicates `git fetch` of a base or feature branch failed.
	ErrGitFetchFailed = errors.New("git fetch failed")
	// ErrGitCheckoutFailed indicates `git checkout` of a feature branch failed.
	ErrGitCheckoutFailed = errors.New("git checkout failed")
	// ErrGitLocalBranchDiverged indicates a local feature branch has commits absent from both
	// the remote feature branch and the base branch.
	ErrGitLocalBranchDiverged = errors.New("local branch has unpushed commits")
	// ErrGitDefaultBranchResolution indicates the remote's default branch could not be resolved.
	ErrGitDefaultBranchResolution = errors.New("remote default branch resolution failed")

	// ErrGitLsRemoteFailed indicates listing refs from a remote Git repository failed.
	ErrGitLsRemoteFailed = errors.New("failed to list refs from remote Git repository")
	// ErrNoVersionsAvailable indicates no versions were found for a source.
	ErrNoVersionsAvailable = errors.New("no versions available")
	// ErrNoVersionsMatchConstraints indicates no available version satisfies the configured constraints.
	ErrNoVersionsMatchConstraints = errors.New("no versions match the configured constraints")
	// ErrInvalidSemverConstraint indicates a constraints.version value is not a valid semver constraint.
	ErrInvalidSemverConstraint = errors.New("invalid semver constraint")
	// ErrVersionRangeConflictsWithConstraints indicates a source declared both a semver-range
	// version: and a constraints.version ceiling. These are mutually exclusive. constraints.version's
	// entire purpose is to bound what `atmos vendor update` can bump an *exact* pin to; that purpose
	// no longer applies once version: is itself already a range.
	ErrVersionRangeConflictsWithConstraints = errors.New("version: is a semver range; constraints.version is also set. These are mutually exclusive; remove one")
	// ErrVendorSourceNotFound indicates a requested component/source was not found in the vendor manifest.
	ErrVendorSourceNotFound = errors.New("vendor source not found")
	// ErrVendorSourceNotGit indicates a vendor source is not a Git repository (unsupported for update/diff).
	ErrVendorSourceNotGit = errors.New("vendor source is not a Git repository")
	// ErrVendorDiffFailed indicates the vendor diff operation failed.
	ErrVendorDiffFailed = errors.New("failed to produce vendor diff")
	// ErrVendorUpdateFailed indicates the vendor update operation failed.
	ErrVendorUpdateFailed = errors.New("failed to update vendor manifest")
	// ErrReadVendorFile indicates a vendor manifest file could not be read.
	ErrReadVendorFile = errors.New("failed to read vendor manifest")
	// ErrParseVendorFile indicates a vendor manifest file could not be parsed.
	ErrParseVendorFile = errors.New("failed to parse vendor manifest")
	// ErrInvalidGitRef indicates a Git ref (tag, branch, or commit) could not be resolved.
	ErrInvalidGitRef = errors.New("invalid Git ref")
	// ErrComponentManifestNotFound indicates no component.yaml/component.yml exists in a component's directory.
	ErrComponentManifestNotFound = errors.New("component vendoring manifest not found")
	// ErrInvalidComponentManifestKind indicates a component.yaml's "kind" is not "ComponentVendorConfig".
	ErrInvalidComponentManifestKind = errors.New("invalid kind in component vendoring manifest; expected ComponentVendorConfig")
	// ErrComponentDirNotFound indicates a component's resolved base directory does not exist.
	ErrComponentDirNotFound = errors.New("component directory does not exist")
	// ErrResolveComponentPath indicates a component's filesystem path could not be resolved
	// (path-joining or symlink resolution failed), distinct from ErrComponentDirNotFound's "the
	// path resolved fine but nothing exists there".
	ErrResolveComponentPath = errors.New("failed to resolve component path")
	// ErrDuplicateVendorComponent indicates the same component name is declared more than once
	// across a vendor manifest and its imports, matching the rejection normal `vendor pull`
	// processing already applies (internal/exec's ErrDuplicateComponentsFound) for the
	// --stack/--tags declared-tags resolution path.
	ErrDuplicateVendorComponent = errors.New("duplicate component declared in vendor manifest")
)

// GitHub Enterprise Server (GHES) endpoint resolution errors.
var (
	// ErrInvalidGitHubEndpointURL indicates a GitHub endpoint environment variable
	// (e.g. GITHUB_SERVER_URL, GITHUB_API_URL, ATMOS_TOOLCHAIN_GITHUB_URL) could not be
	// parsed as an absolute HTTP(S) URL. The resolver falls back to its default rather
	// than failing, so this error is logged at debug level, not surfaced to the user.
	ErrInvalidGitHubEndpointURL = errors.New("invalid GitHub endpoint URL")
)

// Release notes update errors.
var (
	// ErrReleaseTagMissing prevents updating notes without a tag to preserve.
	ErrReleaseTagMissing = errors.New("release tag is missing")
	// ErrReleaseTagMismatch indicates GitHub did not preserve the tag during a notes update.
	ErrReleaseTagMismatch = errors.New("release tag changed during notes update")
)

// Source provisioning, S3 source, and stack-manifest hardening errors (field-test findings, PR #3251).
var (
	// ErrSourceComponentNameInvalid indicates a component name (metadata.component) that cannot be
	// used as a source provisioning destination: absolute, volume-qualified, or escaping the
	// components directory.
	ErrSourceComponentNameInvalid = errors.New("invalid component name for source provisioning")
	// ErrSourceDeleteRefused indicates `source delete` declined to remove a directory it cannot
	// prove the source provisioner created, or that another component owns.
	ErrSourceDeleteRefused = errors.New("refusing to delete source directory")
	// ErrS3SourceRegionMismatch indicates an S3 source bucket lives in a different region than the
	// one used for the request.
	ErrS3SourceRegionMismatch = errors.New("S3 source bucket is in a different region than the request")
	// ErrS3SourceForbidden indicates S3 answered 403 for an object probe: either access is denied
	// or the key does not exist and the principal lacks s3:ListBucket.
	ErrS3SourceForbidden = errors.New("S3 source access was forbidden")
	// ErrS3SourceUnsupportedParam indicates an S3 source URL carries credential query parameters
	// that the Atmos S3 getter never honors.
	ErrS3SourceUnsupportedParam = errors.New("unsupported S3 source authentication parameter")
)

// GitHub mock test-helper errors (tests/testhelpers/httpmock).
var (
	// ErrMockFailWithTimesNegative indicates a test called FailWithTimes with a negative
	// times value. -1 (unlimited failure) is reserved for FailWith/FailWithHeaders; a caller
	// that wants an unlimited failure should call one of those instead of FailWithTimes with
	// a negative count, which would otherwise silently behave as unlimited.
	ErrMockFailWithTimesNegative = errors.New("httpmock: FailWithTimes called with negative times; use FailWith for an unlimited failure")
)

// CI reporter and provider sentinels.
var (
	// ErrCICommentKeyRequired is returned when a comment asks to update an existing comment but names no key to find it by.
	ErrCICommentKeyRequired = errors.New("a comment key is required to update an existing comment")

	// ErrCICommentTargetInvalid is returned when a comment target is not one of auto, pr, or commit.
	ErrCICommentTargetInvalid = errors.New("invalid comment target")

	// ErrCICommitUnknown is returned when a commit comment is requested but the commit SHA is unknown.
	ErrCICommitUnknown = errors.New("commit SHA is unknown")
)

// Automation runtime, standalone script, and custom-command input sentinels.
var (
	// ErrScriptHelpShown is returned by cli.command after the host printed the script's help.
	// It ends the script without running its remaining statements; the engine reports success.
	ErrScriptHelpShown = errors.New("script help shown")

	// ErrLiteralFieldUnsupported is returned when the !literal tag is written on a field that
	// does not support it, so the tag cannot be silently ignored.
	ErrLiteralFieldUnsupported = errors.New("!literal is not supported")

	// ErrScriptInterrupted is returned when an interrupt (Ctrl-C) ended a standalone script
	// after its deferred calls ran. The process exits with the conventional status 130.
	ErrScriptInterrupted = errors.New("script interrupted")
)

// Step library, process cleanup, Git hook, and !starlark sentinels.
var (
	// ErrStarlarkUnsupportedInConfig is returned when `!starlark` appears in atmos.yaml, which only
	// stack manifests evaluate.
	ErrStarlarkUnsupportedInConfig = errors.New("!starlark is not supported in atmos.yaml")

	// ErrHookRecursionLimit is returned when lifecycle hooks trigger themselves more than the
	// allowed number of levels deep.
	ErrHookRecursionLimit = errors.New("hook recursion limit exceeded")
)

// Test step (`type: test`) failure sentinel.
var ErrTestsFailed = errors.New("tests failed")

// Policy evaluation errors.
var (
	ErrPolicyInvalid          = errors.New("invalid policy")
	ErrPolicyInputInvalid     = errors.New("invalid policy input")
	ErrPolicyEngineUnknown    = errors.New("policy engine is not registered")
	ErrPolicyEngineRegistered = errors.New("policy engine is already registered")
	ErrPolicyEvaluation       = errors.New("policy evaluation failed")
	ErrPolicyInputUnavailable = errors.New("required policy input is unavailable or incomplete")
	ErrPolicyDenied           = errors.New("policy denied execution")
	ErrPolicyRequirements     = errors.New("policy requirements are not satisfied")
	ErrPolicyPlan             = errors.New("saved plan policy enforcement failed")
)

// ExitCodeError is a typed error that preserves subcommand exit codes.
// This allows the root command to exit with the same code as the subcommand.
// When Code is 0, it indicates successful completion that should exit cleanly without printing errors.
// This avoids deep exits (os.Exit) which are untestable.
type ExitCodeError struct {
	Code int
	// Silent suppresses themed error rendering: the process exits with Code
	// without printing an error box. Used for terminal-handoff steps (tty,
	// interactive, exec) where, like a shell, a non-zero exit from the child
	// program should propagate the code without Atmos rendering its own error
	// (which would query the terminal and can hang when stdin is contended).
	Silent bool
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("subcommand exited with code %d", e.Code)
}
