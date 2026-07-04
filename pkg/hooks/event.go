package hooks

import "strings"

type HookEvent string

const (
	BeforeTerraformInit            HookEvent = "before.terraform.init"
	AfterTerraformInit             HookEvent = "after.terraform.init"
	AfterTerraformApply            HookEvent = "after.terraform.apply"
	AfterTerraformApplyAggregate   HookEvent = "after.terraform.apply.aggregate"
	BeforeTerraformApply           HookEvent = "before.terraform.apply"
	AfterTerraformPlan             HookEvent = "after.terraform.plan"
	AfterTerraformPlanAggregate    HookEvent = "after.terraform.plan.aggregate"
	BeforeTerraformPlan            HookEvent = "before.terraform.plan"
	BeforeTerraformTest            HookEvent = "before.terraform.test"
	AfterTerraformTest             HookEvent = "after.terraform.test"
	BeforeTerraformDeploy          HookEvent = "before.terraform.deploy"
	AfterTerraformDeploy           HookEvent = "after.terraform.deploy"
	AfterTerraformDestroyAggregate HookEvent = "after.terraform.destroy.aggregate"
	BeforeKubernetesRender         HookEvent = "before.kubernetes.render"
	AfterKubernetesRender          HookEvent = "after.kubernetes.render"
	BeforeKubernetesPlan           HookEvent = "before.kubernetes.plan"
	AfterKubernetesPlan            HookEvent = "after.kubernetes.plan"
	BeforeKubernetesDiff           HookEvent = "before.kubernetes.diff"
	AfterKubernetesDiff            HookEvent = "after.kubernetes.diff"
	BeforeKubernetesApply          HookEvent = "before.kubernetes.apply"
	AfterKubernetesApply           HookEvent = "after.kubernetes.apply"
	BeforeKubernetesDeploy         HookEvent = "before.kubernetes.deploy"
	AfterKubernetesDeploy          HookEvent = "after.kubernetes.deploy"
	BeforeKubernetesDelete         HookEvent = "before.kubernetes.delete"
	AfterKubernetesDelete          HookEvent = "after.kubernetes.delete"
	BeforeKubernetesValidate       HookEvent = "before.kubernetes.validate"
	AfterKubernetesValidate        HookEvent = "after.kubernetes.validate"
	BeforeHelmTemplate             HookEvent = "before.helm.template"
	AfterHelmTemplate              HookEvent = "after.helm.template"
	BeforeHelmDiff                 HookEvent = "before.helm.diff"
	AfterHelmDiff                  HookEvent = "after.helm.diff"
	BeforeHelmApply                HookEvent = "before.helm.apply"
	AfterHelmApply                 HookEvent = "after.helm.apply"
	BeforeHelmDeploy               HookEvent = "before.helm.deploy"
	AfterHelmDeploy                HookEvent = "after.helm.deploy"
	BeforeHelmDelete               HookEvent = "before.helm.delete"
	AfterHelmDelete                HookEvent = "after.helm.delete"
	BeforeHelmfileTemplate         HookEvent = "before.helmfile.template"
	AfterHelmfileTemplate          HookEvent = "after.helmfile.template"
	BeforeHelmfileDiff             HookEvent = "before.helmfile.diff"
	AfterHelmfileDiff              HookEvent = "after.helmfile.diff"
	BeforeHelmfileApply            HookEvent = "before.helmfile.apply"
	AfterHelmfileApply             HookEvent = "after.helmfile.apply"
	BeforeHelmfileSync             HookEvent = "before.helmfile.sync"
	AfterHelmfileSync              HookEvent = "after.helmfile.sync"
	BeforeHelmfileDeploy           HookEvent = "before.helmfile.deploy"
	AfterHelmfileDeploy            HookEvent = "after.helmfile.deploy"
	BeforeHelmfileDestroy          HookEvent = "before.helmfile.destroy"
	AfterHelmfileDestroy           HookEvent = "after.helmfile.destroy"
)

// Normalize returns the canonical form of a HookEvent, collapsing command
// aliases to canonical events. Kubernetes plan maps to diff, and deploy maps to
// apply, so hooks configured for either alias fire regardless of which command
// the user runs.
func (e HookEvent) Normalize() HookEvent {
	switch e {
	case AfterTerraformDeploy:
		return AfterTerraformApply
	case BeforeTerraformDeploy:
		return BeforeTerraformApply
	case AfterKubernetesPlan:
		return AfterKubernetesDiff
	case BeforeKubernetesPlan:
		return BeforeKubernetesDiff
	case AfterKubernetesDeploy:
		return AfterKubernetesApply
	case BeforeKubernetesDeploy:
		return BeforeKubernetesApply
	case AfterHelmDeploy:
		return AfterHelmApply
	case BeforeHelmDeploy:
		return BeforeHelmApply
	case AfterHelmfileDeploy, AfterHelmfileSync:
		return AfterHelmfileApply
	case BeforeHelmfileDeploy, BeforeHelmfileSync:
		return BeforeHelmfileApply
	default:
		return e
	}
}

// IsPostExecution reports whether the event fires after a component command has
// already run, i.e. any "after.*" event (Terraform, Kubernetes, etc.).
// For Terraform specifically, store hooks use this to decide whether to skip
// terraform init when reading outputs: after-events can safely skip init because
// the workdir is already initialized, while before-events must run init because
// the workdir may not be initialized yet.
func (e HookEvent) IsPostExecution() bool {
	return strings.HasPrefix(string(e), "after.")
}
