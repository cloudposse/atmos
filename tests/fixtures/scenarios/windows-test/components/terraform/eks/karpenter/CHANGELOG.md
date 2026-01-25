## Release 1.470.0

Components PR [#1076](https://github.com/cloudposse/terraform-aws-components/pull/1076)

#### Bugfix

- Fixed issues with IAM Policy support for cleaning up `v1alpha` resources.

With the previous release of this component, we encouraged users to delete their `v1alpha` Karpenter resources before
upgrading to `v1beta`. However, certain things, such as EC2 Instance Profiles, would not be deleted by Terraform because
they were created or modified by the Karpenter controller.

To enable the `v1beta` Karpenter controller to clean up these resources, we added a second IAM Policy to the official
Karpenter IAM Policy document. This second policy allows the Karpenter controller to delete the `v1alpha` resources.
However, there were 2 problems with that.

First, the policy was subtly incorrect, and did not, in fact, allow the Karpenter controller to delete all the
resources. This has been fixed.

Second, a long EKS cluster name could cause the Karpenter IRSA's policy to exceed the maximum character limit for an IAM
Policy. This has also been fixed by making the `v1alpha` policy a separate managed policy attached to the Karpenter
controller's role, rather than merging the statements into the `v1beta` policy. This change also avoids potential
conflicts with policy SIDs.

> [!NOTE]
>
> #### Innocuous Changes
>
> Terraform will show IAM Policy changes, including deletion of statements from the existing policy and creation of a
> new policy. This is expected and innocuous. The IAM Policy has been split into 2 to avoid exceeding length limits, but
> the current (`v1beta`) policy remains the same and the now separate (`v1alpha`) policy has been corrected.

## Version 1.445.0

Components [PR #1039](https://github.com/cloudposse/terraform-aws-components/pull/1039)

> [!WARNING]
>
> #### Major Breaking Changes
>
> Karpenter at version v0.33.0 transitioned from the `v1alpha` API to the `v1beta` API with many breaking changes. This
> component (`eks/karpenter`) changed as well, dropping support for the `v1alpha` API and adding support for the
> `v1beta` API. At the same time, the corresponding `eks/karpenter-provisioner` component was replaced with the
> `eks/karpenter-node-pool` component. The old components remain available under the
> [`deprecated/`](https://github.com/cloudposse/terraform-aws-components/tree/main/deprecated) directory.

The full list of changes in Karpenter is too extensive to repeat here. See the
[Karpenter v1beta Migration Guide](https://karpenter.sh/v0.32/upgrading/v1beta1-migration/) and the
[Karpenter Upgrade Guide](https://karpenter.sh/docs/upgrading/upgrade-guide/) for details.

While a zero-downtime upgrade is possible, it is very complex and tedious and Cloud Posse does not support it at this
time. Instead, we recommend you delete your existing Karpenter Provisioner (`karpenter-provisioner`) and Controller
(`karpenter`) deployments, which will scale your cluster to zero and leave all your pods suspended, and then deploy the
new components, which will resume your pods.

Full details of the recommended migration process for these components can be found in the
[Migration Guide](https://github.com/cloudposse/terraform-aws-components/blob/main/modules/eks/karpenter/docs/v1alpha-to-v1beta-migration.md).

If you require a zero-downtime upgrade, please contact
[Cloud Posse professional services](https://cloudposse.com/services/) for assistance.

## Version 1.348.0

Components PR [#868](https://github.com/cloudposse/terraform-aws-components/pull/868)

The `karpenter-crd` helm chart can now be installed alongside the `karpenter` helm chart to automatically manage the
lifecycle of Karpenter CRDs. However since this chart must be installed before the `karpenter` helm chart, the
Kubernetes namespace must be available before either chart is deployed. Furthermore, this namespace should persist
whether or not the `karpenter-crd` chart is deployed, so it should not be installed with that given `helm-release`
resource. Therefore, we've moved namespace creation to a separate resource that runs before both charts. Terraform will
handle that namespace state migration with the `moved` block.

There are several scenarios that may or may not require additional steps. Please review the following scenarios and
follow the steps for your given requirements.

### Upgrading an existing `eks/karpenter` deployment without changes

If you currently have `eks/karpenter` deployed to an EKS cluster and have upgraded to this version of the component, no
changes are required. `var.crd_chart_enabled` will default to `false`.

### Upgrading an existing `eks/karpenter` deployment and deploying the `karpenter-crd` chart

If you currently have `eks/karpenter` deployed to an EKS cluster, have upgraded to this version of the component, do not
currently have the `karpenter-crd` chart installed, and want to now deploy the `karpenter-crd` helm chart, a few
additional steps are required!

First, set `var.crd_chart_enabled` to `true`.

Next, update the installed Karpenter CRDs in order for Helm to automatically take over their management when the
`karpenter-crd` chart is deployed. We have included a script to run that upgrade. Run the `./karpenter-crd-upgrade`
script or run the following commands on the given cluster before deploying the chart. Please note that this script or
commands will only need to be run on first use of the CRD chart.

Before running the script, ensure that the `kubectl` context is set to the cluster where the `karpenter` helm chart is
deployed. In Geodesic, you can usually do this with the `set-cluster` command, though your configuration may vary.

```bash
set-cluster <tenant>-<region>-<stage> terraform
```

Then run the script or commands:

```bash
kubectl label crd awsnodetemplates.karpenter.k8s.aws provisioners.karpenter.sh app.kubernetes.io/managed-by=Helm --overwrite
kubectl annotate crd awsnodetemplates.karpenter.k8s.aws provisioners.karpenter.sh meta.helm.sh/release-name=karpenter-crd --overwrite
kubectl annotate crd awsnodetemplates.karpenter.k8s.aws provisioners.karpenter.sh meta.helm.sh/release-namespace=karpenter --overwrite
```

> [!NOTE]
>
> Previously the `karpenter-crd-upgrade` script included deploying the `karpenter-crd` chart. Now that this chart is
> moved to Terraform, that helm deployment is no longer necessary.
>
> For reference, the `karpenter-crd` chart can be installed with helm with the following:
>
> ```bash
> helm upgrade --install karpenter-crd oci://public.ecr.aws/karpenter/karpenter-crd --version "$VERSION" --namespace karpenter
> ```

Now that the CRDs are upgraded, the component is ready to be applied. Apply the `eks/karpenter` component and then apply
`eks/karpenter-provisioner`.

#### Note for upgrading Karpenter from before v0.27.3 to v0.27.3 or later

If you are upgrading Karpenter from before v0.27.3 to v0.27.3 or later, you may need to run the following command to
remove an obsolete webhook:

```bash
kubectl delete mutatingwebhookconfigurations defaulting.webhook.karpenter.sh
```

See [the Karpenter upgrade guide](https://karpenter.sh/v0.32/upgrading/upgrade-guide/#upgrading-to-v0273) for more
details.

### Upgrading an existing `eks/karpenter` deployment where the `karpenter-crd` chart is already deployed

If you currently have `eks/karpenter` deployed to an EKS cluster, have upgraded to this version of the component, and
already have the `karpenter-crd` chart installed, simply set `var.crd_chart_enabled` to `true` and redeploy Terraform to
have Terraform manage the helm release for `karpenter-crd`.

### Net new deployments

If you are initially deploying `eks/karpenter`, no changes are required, but we recommend installing the CRD chart. Set
`var.crd_chart_enabled` to `true` and continue with deployment.
