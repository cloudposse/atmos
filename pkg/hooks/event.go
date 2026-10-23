package hooks

import "github.com/cloudposse/atmos/pkg/hooks/events"

// HookEvent is a lifecycle event from the shared event registry.
type HookEvent = events.HookEvent

// Lifecycle event names retained here for hook callers.
const (
	BeforeTerraformInit                     = events.BeforeTerraformInit
	AfterTerraformInit                      = events.AfterTerraformInit
	AfterTerraformApply                     = events.AfterTerraformApply
	AfterTerraformApplyAggregate            = events.AfterTerraformApplyAggregate
	BeforeTerraformApply                    = events.BeforeTerraformApply
	BeforeTerraformApplyAggregate           = events.BeforeTerraformApplyAggregate
	AfterTerraformPlan                      = events.AfterTerraformPlan
	AfterTerraformPlanAggregate             = events.AfterTerraformPlanAggregate
	BeforeTerraformPlan                     = events.BeforeTerraformPlan
	BeforeTerraformPlanAggregate            = events.BeforeTerraformPlanAggregate
	BeforeTerraformTest                     = events.BeforeTerraformTest
	AfterTerraformTest                      = events.AfterTerraformTest
	BeforeTerraformOutput                   = events.BeforeTerraformOutput
	AfterTerraformOutput                    = events.AfterTerraformOutput
	BeforeTerraformRefresh                  = events.BeforeTerraformRefresh
	AfterTerraformRefresh                   = events.AfterTerraformRefresh
	BeforeTerraformDeploy                   = events.BeforeTerraformDeploy
	AfterTerraformDeploy                    = events.AfterTerraformDeploy
	AfterTerraformDestroyAggregate          = events.AfterTerraformDestroyAggregate
	BeforeTerraformDestroyAggregate         = events.BeforeTerraformDestroyAggregate
	BeforeKubernetesRender                  = events.BeforeKubernetesRender
	AfterKubernetesRender                   = events.AfterKubernetesRender
	BeforeKubernetesPlan                    = events.BeforeKubernetesPlan
	AfterKubernetesPlan                     = events.AfterKubernetesPlan
	BeforeKubernetesDiff                    = events.BeforeKubernetesDiff
	AfterKubernetesDiff                     = events.AfterKubernetesDiff
	BeforeKubernetesApply                   = events.BeforeKubernetesApply
	AfterKubernetesApply                    = events.AfterKubernetesApply
	BeforeKubernetesDeploy                  = events.BeforeKubernetesDeploy
	AfterKubernetesDeploy                   = events.AfterKubernetesDeploy
	BeforeKubernetesDelete                  = events.BeforeKubernetesDelete
	AfterKubernetesDelete                   = events.AfterKubernetesDelete
	BeforeKubernetesValidate                = events.BeforeKubernetesValidate
	AfterKubernetesValidate                 = events.AfterKubernetesValidate
	BeforeHelmTemplate                      = events.BeforeHelmTemplate
	AfterHelmTemplate                       = events.AfterHelmTemplate
	BeforeHelmDiff                          = events.BeforeHelmDiff
	AfterHelmDiff                           = events.AfterHelmDiff
	AfterHelmPlanAggregate                  = events.AfterHelmPlanAggregate
	BeforeHelmApply                         = events.BeforeHelmApply
	AfterHelmApply                          = events.AfterHelmApply
	AfterHelmApplyAggregate                 = events.AfterHelmApplyAggregate
	BeforeHelmDeploy                        = events.BeforeHelmDeploy
	AfterHelmDeploy                         = events.AfterHelmDeploy
	BeforeHelmDelete                        = events.BeforeHelmDelete
	AfterHelmDelete                         = events.AfterHelmDelete
	BeforeAwsCloudFormationPlan             = events.BeforeAwsCloudFormationPlan
	AfterAwsCloudFormationPlan              = events.AfterAwsCloudFormationPlan
	BeforeAwsCloudFormationDiff             = events.BeforeAwsCloudFormationDiff
	AfterAwsCloudFormationDiff              = events.AfterAwsCloudFormationDiff
	BeforeAwsCloudFormationApply            = events.BeforeAwsCloudFormationApply
	AfterAwsCloudFormationApply             = events.AfterAwsCloudFormationApply
	BeforeAwsCloudFormationDeploy           = events.BeforeAwsCloudFormationDeploy
	AfterAwsCloudFormationDeploy            = events.AfterAwsCloudFormationDeploy
	BeforeAwsCloudFormationDelete           = events.BeforeAwsCloudFormationDelete
	AfterAwsCloudFormationDelete            = events.AfterAwsCloudFormationDelete
	BeforeAwsCloudFormationDriftDetect      = events.BeforeAwsCloudFormationDriftDetect
	AfterAwsCloudFormationDriftDetect       = events.AfterAwsCloudFormationDriftDetect
	BeforeAwsCloudFormationDriftDescribe    = events.BeforeAwsCloudFormationDriftDescribe
	AfterAwsCloudFormationDriftDescribe     = events.AfterAwsCloudFormationDriftDescribe
	BeforeAwsCloudFormationChangesetCreate  = events.BeforeAwsCloudFormationChangesetCreate
	AfterAwsCloudFormationChangesetCreate   = events.AfterAwsCloudFormationChangesetCreate
	BeforeAwsCloudFormationChangesetExecute = events.BeforeAwsCloudFormationChangesetExecute
	AfterAwsCloudFormationChangesetExecute  = events.AfterAwsCloudFormationChangesetExecute
	BeforeHelmfileTemplate                  = events.BeforeHelmfileTemplate
	AfterHelmfileTemplate                   = events.AfterHelmfileTemplate
	BeforeHelmfileDiff                      = events.BeforeHelmfileDiff
	AfterHelmfileDiff                       = events.AfterHelmfileDiff
	BeforeHelmfileApply                     = events.BeforeHelmfileApply
	AfterHelmfileApply                      = events.AfterHelmfileApply
	BeforeHelmfileSync                      = events.BeforeHelmfileSync
	AfterHelmfileSync                       = events.AfterHelmfileSync
	BeforeHelmfileDeploy                    = events.BeforeHelmfileDeploy
	AfterHelmfileDeploy                     = events.AfterHelmfileDeploy
	BeforeHelmfileDestroy                   = events.BeforeHelmfileDestroy
	AfterHelmfileDestroy                    = events.AfterHelmfileDestroy
	BeforeScaffoldGenerate                  = events.BeforeScaffoldGenerate
	AfterScaffoldGenerate                   = events.AfterScaffoldGenerate
)
