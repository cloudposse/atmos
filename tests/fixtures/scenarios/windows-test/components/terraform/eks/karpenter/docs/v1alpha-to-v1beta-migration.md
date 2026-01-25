# Migration Guide

## Prepare to Upgrade Karpenter API version

Before you begin upgrading from Karpenter `v1alpha5` to `v1beta1` APIs, you should get your applications ready for the
changes and validate that your existing configuration has been applied to all your Karpenter instances. You may also
want to upgrade to the latest `v1alpha5` version of Karpenter (0.31.4 as of this writing) to ensure you haven't missed
any changes.

### Validate your existing Karpenter deployments

In order to preserve some kind of ability to rollback, you should validate your existing Karpenter deployments are in a
good state by planning them and either verifying they have no changes, or fixing them or deploying the changes. Then
freeze this configuration so that you can roll back to it if needed.

### Make all your changes to related components

Make all the changes to related components that are required to support the new version of Karpenter. This mainly
involves updating annotations and tolerations in your workloads to match the new Karpenter annotations and taints. Keep
the existing annotations and tolerations in place, so that your workloads will work with both versions.

A lot of labels, tags, and annotations have changed in the new version of Karpenter. You should review the
[Karpenter v1beta1 migration guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/) and roll out the changes to
your workloads before upgrading Karpenter. Where possible, you should roll out the changes in such a way that they work
with both the old and new versions of Karpenter. For example, instead of replacing the old annotations with the new
annotations, you should add the new annotations in addition to the old annotations, and remove the old annotations
later.

Here are some highlights of the changes, but you should review the full
[Karpenter v1beta1 migration guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/) for all the changes:

- Annotations `karpenter.sh/do-not-consolidate` and `karpenter.sh/do-not-evict` have been replaced with
  `karpenter.sh/do-not-disrupt: "true"`
- Nodes spawned by the `v1beta1` resource will use the taint `karpenter.sh/disruption:NoSchedule=disrupting` instead of
  `node.kubernetes.io/unschedulable` so you may need to adjust pod tolerations
- The following deprecated node labels have been removed in favor of their more modern equivalents. These need to be
  changed in your workloads where they are used for topology constraints, affinities, etc. and they also need to be
  changed in your NodePool (formerly Provisioner) requirements:
  - `failure-domain.beta.kubernetes.io/zone` -> `topology.kubernetes.io/zone`
  - `failure-domain.beta.kubernetes.io/region` -> `topology.kubernetes.io/region`
  - `beta.kubernetes.io/arch` -> `kubernetes.io/arch`
  - `beta.kubernetes.io/os` -> `kubernetes.io/os`
  - `beta.kubernetes.io/instance-type` -> `node.kubernetes.io/instance-type`

Deploy all these changes.

### Deploy a managed node group, if you haven't already

Karpenter now recommends deploying to it into a managed node group rather than via Fargate. In part, this is because
Karpenter also strongly recommends it be deployed to the `kube-system` namespace, and deploying the `kube-system`
namespace to Fargate is inefficient at best. This component no longer supports deploying Karpenter to any namespace
other than `kube-system`, so if you had been deploying it to Fargate, you probably want to provision a minimal managed
node group to run the `kube-system` namespace, and it will also host Karpenter as well.

## Migration, the Long Way

It is possible to upgrade Karpenter step-by-step, but it is a long process. Here are the basic steps to get you to
v0.36.0 (there may be more for later versions):

- Upgrade to v0.31.4 (or later v0.31.x if available), fixing any upgrade issues
- Upgrade to v0.32.9, moving Karpenter to the `kube-system` namespace, which will require some manual intervention when
  applying the Helm chart
- Deploy all new Karpenter `v1beta1` resources that mirror your `v1alpha5` resources, and make all the other changes
  listed in the [v1beta1 migration guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/) such as (not a
  complete list):
  - Annotations `karpenter.sh/do-not-consolidate` and `karpenter.sh/do-not-evict` have been replaced with
    `karpenter.sh/do-not-disrupt: "true"`
  - Karpenter-generated tag keys have changed, so you may need to adjust your IAM Policies if you are using
    Attribute-Based Access Control.
  - The `karpenter-global-settings` ConfigMap has been replaced with settings via Environment Variables and CLI flags
  - Default log encoding changed from console to JSON, so if your log processing cannot handle JSON logs, you should
    probably change your log processing rather than sticking with the deprecated console encoding
  - Prometheus metrics are now served on port 8001. You may need to adjust scraper configurations, and you may need to
    override this port setting if it would otherwise cause a conflict.
- Delete all old Karpenter `v1alpha5` resources
- Review the [Karpenter upgrade guide](https://karpenter.sh/docs/upgrading/upgrade-guide/) and make additional changes
  to reflect your preferences regarding new features and changes in behavior, such as (not a complete list):
  - Availability of Node Pool Disruption Budgets
  - Incompatibility with Ubuntu 22.04 EKS AMI
  - Changes to names of Kubernetes labels Karpenter uses
  - Changes to tags Karpenter uses
  - Recommendation to move Karpenter from `karpenter` namespace to `kube-system`
  - Deciding on if you want drift detection enabled
  - Changes to logging configuration
  - Changes to how Selectors, e.g. for Subnets, are configured
  - Karpenter now uses a podSecurityContext to configure the `fsgroup` for pod volumes (to `65536`), which can affect
    sidecars
- Upgrade to the latest version of Karpenter

This multistep process is particularly difficult to organize and execute using Terraform and Helm because of the
changing resource types and configuration required to support both `v1alpha5` and `v1beta1` resources at the same time.
Therefore, this component does not support this path, and this document does not describe it in any greater detail.

## Migration, the Shorter Way

The shortest way is to delete all Karpenter resources, completely deleting the Cloud Posse `eks/karpenter` and
`eks/karpenter-provisioner` components, and then upgrading the components to the latest version and redeploying them.

The shorter (but not shortest) way is to abandon the old configuration and code in place, taking advantage of the fact
that `eks/karpenter-provisioner` has been replaced with `eks/karpenter-node-pool`. That path is what the rest of this
document describes.

### Disable automatic deployments

If you are using some kind of automatic deployment, such as Spacelift, disable it for the `karpenter` and
`karpenter-provisioner` stacks. This is because we will roll out breaking changes, and want to sequence the operations
manually. If using Spacelift, you can disable it by setting `workspace_enabled: false`, but remember, you must check in
the changes and merge them to your default branch in order for them to take effect.

### Copy existing configuration to new names

The `eks/karpenter-provisioner` component has been replaced with the `eks/karpenter-node-pool` component. You should
copy your existing `karpenter-provisioner` stacks to `karpenter-node-pool` stacks, adjusting the component name and
adding it to the import list wherever `karpenter-provisioner` was imported.

For the moment, we will leave the old `karpenter-provisioner` component and stacks in place.

### Revise your copied `karpenter-node-pool` stacks

Terminology has changed and some settings have been moved in the new version. See the
[Karpenter v1beta1 Migration Guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/) for details.

For the most part you can just use the copied settings from the old version of this component directly in new version,
but there are some changes.

As you have seen, "provisioner" has been renamed "node_pool". So you will need to make some changes to your new
`karpenter-node-pool` stacks.

Specifically, `provisioner` input has been renamed `node_pools`. Within that input:

- The `consolidation` input, which used to be a single boolean, has been replaced with the full `disruption` element of
  the NodePool.
- The old `ttl_seconds_after_empty` is now `disruption.consolidate_after`.
- The old `ttl_seconds_until_expired` is now `disruption.max_instance_lifetime` to align with the EC2 Auto Scaling Group
  terminology, although Karpenter calles it `expiresAfter`.
- `spec.template.spec.kubelet` settings are not yet supported by this component.
- `settings.aws.enablePodENI` and `settings.aws.enableENILimitedPodDensity`, which you may have previously set via
  `chart_values`, have been dropped by Karpenter.
- Many other chart values you may be been setting by `chart_values` have been moved. See
  [Karpenter v1beta1 Migration Guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/#helm-values) for details.

### Revise your `karpenter` stacks

The `karpenter` stack probably requires only a few changes. In general, if you had been setting anything via
`chart_values`, you probably should just delete those settings. If the component doesn't support the setting, it is
likely that Karpenter no longer supports it, or the way it is configured vai the chart has changed.

For examples, `AWS_ENI_LIMITED_POD_DENSITY` is no longer supported by Karpenter, and `replicas` is now a setting of the
component, and does not need to be set via `chart_values`.

- Update the chart version. Find the latest version by looking inside the
  [Chart.yaml](https://github.com/aws/karpenter-provider-aws/blob/main/charts/karpenter/Chart.yaml) file in the
  Karpenter Helm chart repository, on the main branch. Use the value set as `version` (not `appVersion`, if different)
  in that file.

- Karpenter is now always deployed to the `kube-system` namespace. Any Kubernetes namespace configuration inputs have
  been removed. Remove these lines from your configuration:

  ```yaml
  create_namespace: true
  kubernetes_namespace: "karpenter"
  ```

- The number of replicas can now be set via the `replicas` input. That said, there is little reason to change this from
  the default of 2. Only one controller is active at a time, and the other one is a standby. There is no load sharing or
  other reason to have more than 2 replicas in most cases.

- The lifecycle settings `consolidation`, `ttl_seconds_after_empty` and `ttl_seconds_until_expired` have been moved to
  the `disruption` input. Unfortunately, the documentation for the Karpetner Disruption spec is lacking, so read the
  comments in the code for the `disruption` input for details. The short story is:

  - `consolidation` is now enabled by default. To disable it, set `disruption.consolidate_after` to `"Never"`.
  - If you previously set `ttl_seconds_after_empty`, move that setting to the `disruption.consolidate_after` attribute,
    and set `disruption.consolidation_policy` to `"WhenEmpty"`.
  - If you previously set `ttl_seconds_until_expired`, move that setting to the `disruption.max_instance_lifetime`
    attribute. If you previously left it unset, you can keep the previous behavior by setting it to "Never". The new
    default it to expire instances after 336 hours (14 days).
  - The disruption setting can optionally take a list of `budget` settings. See the
    [Disruption Budgets documentation](https://karpenter.sh/docs/concepts/disruption/#disruption-budgets) for details on
    what this is. It is **not** the same as a Pod disruption budget, which tries to put limits on the number of
    instances of a pod that are running at once. Instead, it is a limitation on how quickly Karpenter will remove
    instances.

- The [interruption handler](https://karpenter.sh/docs/concepts/disruption/#interruption) is now enabled by default. If
  you had disabled it, you may want to reconsider. It is a key feature of Karpenter that allows it to automatically
  handle interruptions and reschedule pods on other nodes gracefully given the advance notice provided by AWS of
  involuntary interruption events.

- The `legacy_create_karpenter_instance_profile` has been removed. Previously, this component would create an instance
  profile for the Karpenter nodes. This flag disabled that behavior in favor of having the EKS cluster create the
  instance profile, because the Terraform code could not handle certain edge cases. Now Karpenter itself creates the
  instance profile and handles the edge cases, so the flag is no longer needed.

  As a side note: if you are using the `eks/cluster` component, you can remove any
  `legacy_do_not_create_karpenter_instance_profile` configuration from it after finishing the migration to the new
  Karpenter APIs.

- Logging configuration has changed. The component has a single `logging` input object that defaults to enabled at the
  "info" level for the controller. If you were configuring logging via `chart_values`, we recommend you remove that
  configuration and use the new input object. However, if the input object is not sufficient for your needs, you can use
  new chart values to configure the logging level and format, but be aware the new chart inputs controlling logging are
  significantly different from the old ones.

- You may want to take advantage of the new `batch_idle_duration` and `batch_max_duration` settings, set as attributes
  of the `settings` input. These settings allow you to control how long Karpenter waits for more pods to be deployed
  before launching a new instance. This is useful if you have many pods to deploy in response to a single event, such as
  when launching multiple CI jobs to handle a new release. Karpenter can then launch a single instance to handle them
  all, rather than launching a new instance for each pod. See the
  [batching parameters](https://karpenter.sh/docs/reference/settings/#batching-parameters) documentation for details.
