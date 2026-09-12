package cloudformation

import "strings"

// An operationHelpEntry holds a command's Long help text and Example block,
// mirroring the corresponding website/docs/cli/commands/aws/cloudformation/*.mdx
// page so `--help` output and the docs stay consistent.
type operationHelpEntry struct {
	long    string
	example string
}

// operationHelpBySubCommand maps each Operation-dispatch identifier to its
// help text. Keyed by subCommand rather than use, since most operations only
// have one use; "apply" (apply/deploy) and "diff" (diff/plan) are aliased and
// get a use-specific override in operationHelpText.
var operationHelpBySubCommand = map[string]operationHelpEntry{
	"stackset-create": {
		long: "Create a CloudFormation StackSet (CreateStackSet) from the resolved\n" +
			"kind: aws/stackset provision target's accounts/regions/permission_model/role\n" +
			"settings, plus the component's own template/parameters/capabilities/tags.\n" +
			"When the target declares both accounts and regions, stackset create also\n" +
			"creates the initial stack instances (CreateStackInstances) across that\n" +
			"account/region matrix and waits for the operation to finish -- the only\n" +
			"verb that creates instances.",
		example: "  atmos aws cloudformation stackset create vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation stackset create vpc --stack plat-ue2-dev --auto-approve\n" +
			"  atmos aws cloudformation stackset create vpc --stack plat-ue2-dev --target multi-account",
	},
	"stackset-update": {
		long: "Update a CloudFormation StackSet's template, parameters, and capabilities\n" +
			"(UpdateStackSet) from the component's current configuration, and wait for\n" +
			"the update to propagate to every existing stack instance. stackset update\n" +
			"never changes which accounts/regions have instances -- that set is fixed at\n" +
			"stackset create time.",
		example: "  atmos aws cloudformation stackset update vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation stackset update vpc --stack plat-ue2-dev --auto-approve\n" +
			"  atmos aws cloudformation stackset update vpc --stack plat-ue2-dev --target multi-account",
	},
	"stackset-delete": {
		long: "Delete a CloudFormation StackSet. CloudFormation requires every stack\n" +
			"instance to be removed before the StackSet itself can be deleted, so\n" +
			"stackset delete lists the StackSet's current instances, deletes them all\n" +
			"(DeleteStackInstances, retaining no resources) if any exist, waits for that\n" +
			"operation to finish, and only then deletes the StackSet (DeleteStackSet).",
		example: "  atmos aws cloudformation stackset delete vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation stackset delete vpc --stack plat-ue2-dev --auto-approve\n" +
			"  atmos aws cloudformation stackset delete --all --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation stackset delete --affected --base origin/main",
	},
	"stackset-instances": {
		long: "List a CloudFormation StackSet's stack instances (ListStackInstances) --\n" +
			"each instance's account, region, status, and stack ID.",
		example: "  atmos aws cloudformation stackset instances vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation stackset instances --all --stack plat-ue2-dev",
	},
	"tree": {
		long: "Render the deployed stack's nested-stack dependency tree: walk the stack's\n" +
			"resources, recursing into every AWS::CloudFormation::Stack resource, up to\n" +
			"10 levels deep.",
		example: "  atmos aws cloudformation tree vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation tree --all --stack plat-ue2-dev",
	},
	"logs": {
		long: "Show the combined CloudFormation event log across a stack and every nested\n" +
			"stack beneath it (up to 10 levels deep), merged into a single chronological\n" +
			"timeline -- instead of having to check each nested stack's events\n" +
			"separately.",
		example: "  atmos aws cloudformation logs vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation logs vpc --stack plat-ue2-dev --chart",
	},
	"watch": {
		long: "Attach to a stack's operation and stream its events until the stack reaches\n" +
			"a terminal status -- whether that operation is currently in progress,\n" +
			"already finished, or was started outside Atmos entirely (the AWS Console, a\n" +
			"CI pipeline running raw aws cloudformation, another teammate's terminal).\n" +
			"This is distinct from apply/deploy/delete's automatic inline streaming,\n" +
			"which only covers the operation that command itself just started.",
		example: "  atmos aws cloudformation watch vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation watch --all --stack plat-ue2-dev",
	},
	"render": {
		long: "Render the component's local template -- resolved from `template:` and, when\n" +
			"`source:` is set, JIT-provisioned first -- without calling any AWS API.\n" +
			"render does not authenticate, so it works offline and without AWS credentials.",
		example: "  atmos aws cloudformation render vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation render --all --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation render --affected --base origin/main",
	},
	"diff": {
		long: "Create (or reuse) a CloudFormation changeset for the component and render the\n" +
			"predicted changes, without executing it. diff authenticates and calls the\n" +
			"CloudFormation API -- it previews what apply would actually do to the live\n" +
			"stack, unlike render, which never leaves the local template.",
		example: "  atmos aws cloudformation diff vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation diff --all --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation diff --affected --base origin/main",
	},
	subCommandApply: {
		long: "Create or update the CloudFormation stack for a component: create (or reuse)\n" +
			"a changeset and execute it. After a successful apply, Atmos applies the\n" +
			"component's stack_policy (if set) and renders the stack's Outputs.",
		example: "  atmos aws cloudformation apply vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation apply vpc --stack plat-ue2-dev --auto-approve\n" +
			"  atmos aws cloudformation apply --all --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation apply --affected --base origin/main",
	},
	subCommandDelete: {
		long: "Delete the CloudFormation stack for a component (DeleteStack), then stream\n" +
			"stack events until the stack is gone. Termination protection is always\n" +
			"respected -- Atmos never disables it silently.",
		example: "  atmos aws cloudformation delete vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation delete vpc --stack plat-ue2-dev --auto-approve\n" +
			"  atmos aws cloudformation delete vpc --stack plat-ue2-dev --retain-resources=SecurityGroup,ManualDbSnapshot\n" +
			"  atmos aws cloudformation delete vpc --stack plat-ue2-dev --disable-termination-protection",
	},
	"validate": {
		long: "Validate the component's template with CloudFormation's server-side\n" +
			"ValidateTemplate API -- a syntax and capability-discovery check, not a local\n" +
			"linter.",
		example: "  atmos aws cloudformation validate vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation validate --all --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation validate --affected --base origin/main",
	},
	"output": {
		long: "Show the deployed stack's Outputs (DescribeStacks), formatted for\n" +
			"consumption by shells, other tools, or other Atmos components. apply\n" +
			"renders this same view automatically at the end of a successful deploy.",
		example: "  atmos aws cloudformation output vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation output vpc --stack plat-ue2-dev --format=json\n" +
			"  atmos aws cloudformation output vpc --stack plat-ue2-dev --format=dotenv --flatten --uppercase",
	},
	"fmt": {
		long: "Format the component's local template in place: a dependency-free, native\n" +
			"round-trip through the YAML parser that re-serializes the template with\n" +
			"consistent indentation, preserving comments and key order.",
		example: "  atmos aws cloudformation fmt vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation fmt vpc --stack plat-ue2-dev --check\n" +
			"  atmos aws cloudformation fmt --all --stack plat-ue2-dev",
	},
	"changeset-create": {
		long: "Create a CloudFormation changeset for the component and leave it in place for\n" +
			"later manual review and execution -- the explicit-control complement to\n" +
			"diff/plan's implicit, preview-only changeset.",
		example: "  atmos aws cloudformation changeset create vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation changeset create --all --stack plat-ue2-dev",
	},
	"changeset-execute": {
		long: "Execute a previously-created, named changeset (ExecuteChangeSet) and stream\n" +
			"stack events until the operation reaches a terminal state -- unlike apply,\n" +
			"which creates (or reuses) and executes a changeset in one step, changeset\n" +
			"execute acts on an existing changeset by name.",
		example: "  atmos aws cloudformation changeset execute vpc --stack plat-ue2-dev --changeset-name vpc-2026-08-25\n" +
			"  atmos aws cloudformation changeset execute vpc --stack plat-ue2-dev --changeset-name vpc-2026-08-25 --auto-approve",
	},
	"changeset-list": {
		long: "List a CloudFormation stack's changesets (ListChangeSets), newest first --\n" +
			"each entry's name, status, and description.",
		example: "  atmos aws cloudformation changeset list vpc --stack plat-ue2-dev",
	},
	"changeset-delete": {
		long: "Delete a named changeset (DeleteChangeSet) without touching the stack\n" +
			"itself. Use this to clean up changesets created with changeset create that\n" +
			"you decided not to execute.",
		example: "  atmos aws cloudformation changeset delete vpc --stack plat-ue2-dev --changeset-name vpc-2026-08-25",
	},
	"drift-detect": {
		long: "Trigger a fresh CloudFormation drift detection (DetectStackDrift) against\n" +
			"the deployed stack and poll until it completes, then render the overall\n" +
			"drift status and drifted-resource count.",
		example: "  atmos aws cloudformation drift detect vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation drift detect vpc --stack plat-ue2-dev --fail-on-drift",
	},
	"drift-describe": {
		long: "Show the per-resource results of the most recently completed drift\n" +
			"detection (DescribeStackResourceDrifts) without triggering a new one. Run\n" +
			"drift detect first -- this command only reads existing results.",
		example: "  atmos aws cloudformation drift describe vpc --stack plat-ue2-dev",
	},
	"get-template": {
		long: "Fetch the deployed stack's template (GetTemplate) and write it to stdout.\n" +
			"By default this is the fully-processed template CloudFormation actually\n" +
			"deployed, not the user-submitted source.",
		example: "  atmos aws cloudformation get template vpc --stack plat-ue2-dev\n" +
			"  atmos aws cloudformation get template vpc --stack plat-ue2-dev --original",
	},
	"get-policy": {
		long: "Fetch the deployed stack's current stack policy (GetStackPolicy) and write\n" +
			"it to stdout -- the policy CloudFormation is actually enforcing, as opposed\n" +
			"to the stack_policy configured on the component.",
		example: "  atmos aws cloudformation get policy vpc --stack plat-ue2-dev",
	},
}

// operationHelpText returns the Long/Example help text for one
// newOperationCommand registration, applying use-specific overrides for the
// "deploy" and "plan" aliases (which share a subCommand with "apply"/"diff"
// but differ slightly in wording).
func operationHelpText(use, subCommand string) (string, string) {
	entry, ok := operationHelpBySubCommand[subCommand]
	if !ok {
		return "", ""
	}
	switch use {
	case "deploy":
		return "Create or update the CloudFormation stack for a component. deploy is an\n" +
				"alias for apply that defaults --auto-approve to true, matching the\n" +
				"established `terraform deploy` = \"apply with auto-approve\" convention --\n" +
				"useful for CI, where nothing is present to answer an interactive prompt.",
			strings.ReplaceAll(entry.example, "apply ", "deploy ")
	case "plan":
		return "Preview changes an apply would make to a CloudFormation stack. plan is an\n" +
				"alias for diff: it creates (or reuses) a changeset and renders the\n" +
				"predicted changes without executing it.",
			strings.ReplaceAll(entry.example, "diff ", "plan ")
	default:
		return entry.long, entry.example
	}
}
