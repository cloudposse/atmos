---
tags:
  - component/eks/karpenter
  - layer/eks
  - provider/aws
  - provider/helm
---

# Component: `eks-karpenter-controller`

This component provisions [Karpenter](https://karpenter.sh) on an EKS cluster.
It requires at least version 0.32.0 of Karpenter, though using the latest
version is recommended.
## Usage

**Stack Level**: Regional

These instructions assume you are provisioning 2 EKS clusters in the same account and region, named "blue" and "green",
and alternating between them. If you are only using a single cluster, you can ignore the "blue" and "green" references
and remove the `metadata` block from the `karpenter` module.

```yaml
components:
  terraform:
    # Base component of all `karpenter` components
    eks/karpenter:
      metadata:
        type: abstract
      vars:
        enabled: true
        eks_component_name: "eks/cluster"
        name: "karpenter"
        # https://github.com/aws/karpenter/tree/main/charts/karpenter
        chart_repository: "oci://public.ecr.aws/karpenter"
        chart: "karpenter"
        chart_version: "1.6.0"
        # Enable Karpenter to get advance notice of spot instances being terminated
        # See https://karpenter.sh/docs/concepts/#interruption
        interruption_handler_enabled: true
        resources:
          limits:
            cpu: "300m"
            memory: "1Gi"
          requests:
            cpu: "100m"
            memory: "512Mi"
        cleanup_on_fail: true
        atomic: true
        wait: true
        # "karpenter-crd" can be installed as an independent helm chart to manage the lifecycle of Karpenter CRDs
        crd_chart_enabled: true
        crd_chart: "karpenter-crd"
        # replicas set the number of Karpenter controller replicas to run
        replicas: 2
        # "settings" controls a subset of the settings for the Karpenter controller regarding batch idle and max duration.
        # you can read more about these settings here: https://karpenter.sh/docs/reference/settings/
        settings:
          batch_idle_duration: "1s"
          batch_max_duration: "10s"
        # (Optional) "settings" which do not have an explicit mapping and may be subject to change between helm chart versions
        additional_settings:
          featureGates:
            nodeRepair: false
            reservedCapacity: true
            spotToSpotConsolidation: true
        # The logging settings for the Karpenter controller
        logging:
          enabled: true
          level:
            controller: "info"
            global: "info"
            webhook: "error"
```

## Provision Karpenter on EKS cluster

Here we describe how to provision Karpenter on an EKS cluster. We will be using the `plat-ue2-dev` stack as an example.

### Provision Service-Linked Roles for EC2 Spot and EC2 Spot Fleet

Note: If you want to use EC2 Spot for the instances launched by Karpenter, you may need to provision the following
Service-Linked Role for EC2 Spot:

- Service-Linked Role for EC2 Spot

This is only necessary if this is the first time you're using EC2 Spot in the account. Since this is a one-time
operation, we recommend you do this manually via the AWS CLI:

```bash
aws --profile <namespace>-<tenamt>-gbl-<stage>-admin iam create-service-linked-role --aws-service-name spot.amazonaws.com
```

Note that if the Service-Linked Roles already exist in the AWS account (if you used EC2 Spot or Spot Fleet before), and
you try to provision them again, you will see the following errors:

```text
An error occurred (InvalidInput) when calling the CreateServiceLinkedRole operation:
Service role name AWSServiceRoleForEC2Spot has been taken in this account, please try a different suffix
```

For more details, see:

- https://docs.aws.amazon.com/batch/latest/userguide/spot_fleet_IAM_role.html
- https://docs.aws.amazon.com/IAM/latest/UserGuide/using-service-linked-roles.html

The process of provisioning Karpenter on an EKS cluster consists of 3 steps.

### 1. Provision EKS IAM Role for Nodes Launched by Karpenter

> [!NOTE]
>
> #### VPC assumptions being made
>
> We assume you've already created a VPC using our [VPC component](/modules/vpc) and have private subnets already set
> up. The Karpenter node pools will be launched in the private subnets.

EKS IAM Role for Nodes launched by Karpenter are provisioned by the `eks/cluster` component. (EKS can also provision a
Fargate Profile for Karpenter, but deploying Karpenter to Fargate is not recommended.):

```yaml
components:
  terraform:
    eks/cluster-blue:
      metadata:
        component: eks/cluster
        inherits:
          - eks/cluster
      vars:
        karpenter_iam_role_enabled: true
```

> [!NOTE]
>
> The AWS Auth API for EKS is used to authorize the Karpenter controller to interact with the EKS cluster.

Karpenter is installed using a Helm chart. The Helm chart installs the Karpenter controller and a webhook pod as a
Deployment that needs to run before the controller can be used for scaling your cluster. We recommend a minimum of one
small node group with at least one worker node.

As an alternative, you can run these pods on EKS Fargate by creating a Fargate profile for the karpenter namespace.
Doing so will cause all pods deployed into this namespace to run on EKS Fargate. Do not run Karpenter on a node that is
managed by Karpenter.

See
[Run Karpenter Controller...](https://aws.github.io/aws-eks-best-practices/karpenter/#run-the-karpenter-controller-on-eks-fargate-or-on-a-worker-node-that-belongs-to-a-node-group)
for more details.

We provision IAM Role for Nodes launched by Karpenter because they must run with an Instance Profile that grants
permissions necessary to run containers and configure networking.

We define the IAM role for the Instance Profile in `components/terraform/eks/cluster/controller-policy.tf`.

Note that we provision the EC2 Instance Profile for the Karpenter IAM role in the `components/terraform/eks/karpenter`
component (see the next step).

Run the following commands to provision the EKS Instance Profile for Karpenter and the IAM role for instances launched
by Karpenter on the blue EKS cluster and add the role ARNs to the EKS Auth API:

```bash
atmos terraform plan eks/cluster-blue -s plat-ue2-dev
atmos terraform apply eks/cluster-blue -s plat-ue2-dev
```

For more details, refer to:

- [Getting started with Terraform](https://aws-ia.github.io/terraform-aws-eks-blueprints/getting-started/)
- [Getting started with `eksctl`](https://karpenter.sh/docs/getting-started/getting-started-with-karpenter/)

### 2. Provision `karpenter` component

In this step, we provision the `components/terraform/eks/karpenter` component, which deploys the following resources:

- Karpenter CustomerResourceDefinitions (CRDs) using the Karpenter CRD Chart and the `helm_release` Terraform resource
- Karpenter Kubernetes controller using the Karpenter Helm Chart and the `helm_release` Terraform resource
- EKS IAM role for Kubernetes Service Account for the Karpenter controller (with all the required permissions)
- An SQS Queue and Event Bridge rules for handling Node Interruption events (i.e. Spot)

Create a stack config for the blue Karpenter component in `stacks/catalog/eks/clusters/blue.yaml`:

```yaml
eks/karpenter-blue:
  metadata:
    component: eks/karpenter
    inherits:
      - eks/karpenter
  vars:
    eks_component_name: eks/cluster-blue
```

Run the following commands to provision the Karpenter component on the blue EKS cluster:

```bash
atmos terraform plan eks/karpenter-blue -s plat-ue2-dev
atmos terraform apply eks/karpenter-blue -s plat-ue2-dev
```

### 3. Provision `karpenter-node-pool` component

In this step, we provision the `components/terraform/eks/karpenter-node-pool` component, which deploys Karpenter
[NodePools](https://karpenter.sh/v0.36/getting-started/getting-started-with-karpenter/#5-create-nodepool) using the
`kubernetes_manifest` resource.

> [!TIP]
>
> #### Why use a separate component for NodePools?
>
> We create the NodePools as a separate component since the CRDs for the NodePools are created by the Karpenter
> component. This helps manage dependencies.

First, create an abstract component for the `eks/karpenter-node-pool` component:

```yaml
components:
  terraform:
    eks/karpenter-node-pool:
      metadata:
        type: abstract
      vars:
        enabled: true
        # Disabling Manifest Experiment disables stored metadata with Terraform state
        # Otherwise, the state will show changes on all plans
        helm_manifest_experiment_enabled: false
        node_pools:
          default:
            # Whether to place EC2 instances launched by Karpenter into VPC private subnets. Set it to `false` to use public subnets
            private_subnets_enabled: true
            # You can use disruption to set the maximum instance lifetime for the EC2 instances launched by Karpenter.
            # You can also configure how fast or slow Karpenter should add/remove nodes.
            # See more: https://karpenter.sh/v0.36/concepts/disruption/
            disruption:
              max_instance_lifetime: "336h" # 14 days
            # Taints can be used to prevent pods without the right tolerations from running on this node pool.
            # See more: https://karpenter.sh/v0.36/concepts/nodepools/#taints
            taints: []
            total_cpu_limit: "1k"
            # Karpenter node pool total memory limit for all pods running on the EC2 instances launched by Karpenter
            total_memory_limit: "1200Gi"
            # Set acceptable (In) and unacceptable (Out) Kubernetes and Karpenter values for node provisioning based on
            # Well-Known Labels and cloud-specific settings. These can include instance types, zones, computer architecture,
            # and capacity type (such as AWS spot or on-demand).
            # See https://karpenter.sh/v0.36/concepts/nodepools/#spectemplatespecrequirements for more details
            requirements:
              - key: "karpenter.sh/capacity-type"
                operator: "In"
                # See https://karpenter.sh/docs/concepts/nodepools/#capacity-type
                # Allow fallback to on-demand instances when spot instances are unavailable
                # By default, Karpenter uses the "price-capacity-optimized" allocation strategy
                # https://aws.amazon.com/blogs/compute/introducing-price-capacity-optimized-allocation-strategy-for-ec2-spot-instances/
                # It is currently not configurable, but that may change in the future.
                # See https://github.com/aws/karpenter-provider-aws/issues/1240
                values:
                  - "on-demand"
                  - "spot"
              - key: "kubernetes.io/os"
                operator: "In"
                values:
                  - "linux"
              - key: "kubernetes.io/arch"
                operator: "In"
                values:
                  - "amd64"
              # The following two requirements pick instances such as c3 or m5
              - key: karpenter.k8s.aws/instance-category
                operator: In
                values: ["c", "m", "r"]
              - key: karpenter.k8s.aws/instance-generation
                operator: Gt
                values: ["2"]
```

Now, create the stack config for the blue Karpenter NodePool component in `stacks/catalog/eks/clusters/blue.yaml`:

```yaml
eks/karpenter-node-pool/blue:
  metadata:
    component: eks/karpenter-node-pool
    inherits:
      - eks/karpenter-node-pool
  vars:
    eks_component_name: eks/cluster-blue
```

Finally, run the following commands to deploy the Karpenter NodePools on the blue EKS cluster:

```bash
atmos terraform plan eks/karpenter-node-pool/blue -s plat-ue2-dev
atmos terraform apply eks/karpenter-node-pool/blue -s plat-ue2-dev
```

## Node Interruption

Karpenter also supports listening for and responding to Node Interruption events. If interruption handling is enabled,
Karpenter will watch for upcoming involuntary interruption events that would cause disruption to your workloads. These
interruption events include:

- Spot Interruption Warnings
- Scheduled Change Health Events (Maintenance Events)
- Instance Terminating Events
- Instance Stopping Events

> [!TIP]
>
> #### Interruption Handler vs. Termination Handler
>
> The Node Interruption Handler is not the same as the Node Termination Handler. The latter is always enabled and
> cleanly shuts down the node in 2 minutes in response to a Node Termination event. The former gets advance notice that
> a node will soon be terminated, so it can have 5-10 minutes to shut down a node.

For more details, see refer to the [Karpenter docs](https://karpenter.sh/v0.32/concepts/disruption/#interruption) and
[FAQ](https://karpenter.sh/v0.32/faq/#interruption-handling)

To enable Node Interruption handling, set `var.interruption_handler_enabled` to `true`. This will create an SQS queue
and a set of Event Bridge rules to deliver interruption events to Karpenter.

## Custom Resource Definition (CRD) Management

Karpenter ships with a few Custom Resource Definitions (CRDs). In earlier versions of this component, when installing a
new version of the `karpenter` helm chart, CRDs were not be upgraded at the same time, requiring manual steps to upgrade
CRDs after deploying the latest chart. However Karpenter now supports an additional, independent helm chart for CRD
management. This helm chart, `karpenter-crd`, can be installed alongside the `karpenter` helm chart to automatically
manage the lifecycle of these CRDs.

To deploy the `karpenter-crd` helm chart, set `var.crd_chart_enabled` to `true`. (Installing the `karpenter-crd` chart
is recommended. `var.crd_chart_enabled` defaults to `false` to preserve backward compatibility with older versions of
this component.)

## EKS Cluster Configuration

This component supports two methods for obtaining EKS cluster information, controlled by the
`account_map_enabled` variable:

1. **Direct Variables (Recommended)**: Set `account_map_enabled: false` and provide EKS cluster details via the `eks` object variable
2. **Internal Remote State (Default)**: Set `account_map_enabled: true` (default) to fetch EKS cluster details from Terraform remote state using `eks_component_name`

### Using Atmos State Functions (Recommended)

When using [Atmos](https://atmos.tools), you can use the `!terraform.state` function to read
EKS cluster outputs from another component's Terraform state and pass them as variables.

```yaml
components:
  terraform:
    eks/karpenter:
      vars:
        enabled: true
        name: "karpenter"
        account_map_enabled: false
        eks:
          eks_cluster_id: !terraform.state eks/cluster eks_cluster_id
          eks_cluster_arn: !terraform.state eks/cluster eks_cluster_arn
          eks_cluster_endpoint: !terraform.state eks/cluster eks_cluster_endpoint
          eks_cluster_certificate_authority_data: !terraform.state eks/cluster eks_cluster_certificate_authority_data
          eks_cluster_identity_oidc_issuer: !terraform.state eks/cluster eks_cluster_identity_oidc_issuer
          karpenter_iam_role_arn: !terraform.state eks/cluster karpenter_iam_role_arn
        chart_repository: "oci://public.ecr.aws/karpenter"
        chart: "karpenter"
        chart_version: "1.6.0"
```

For more information on `!terraform.state`, see the
[Atmos documentation](https://atmos.tools/core-concepts/stacks/templating/functions/terraform.state/).

### Direct Variables Approach

You can also provide EKS cluster information directly:

```yaml
components:
  terraform:
    eks/karpenter:
      vars:
        enabled: true
        name: "karpenter"
        account_map_enabled: false
        eks:
          eks_cluster_id: "my-eks-cluster"
          eks_cluster_arn: "arn:aws:eks:us-east-1:123456789012:cluster/my-eks-cluster"
          eks_cluster_endpoint: "https://ABCDEF1234567890.gr7.us-east-1.eks.amazonaws.com"
          eks_cluster_certificate_authority_data: "LS0tLS1CRUdJTi..."
          eks_cluster_identity_oidc_issuer: "https://oidc.eks.us-east-1.amazonaws.com/id/ABCDEF1234567890"
          karpenter_iam_role_arn: "arn:aws:iam::123456789012:role/my-eks-cluster-karpenter-node"
        chart_repository: "oci://public.ecr.aws/karpenter"
        chart: "karpenter"
        chart_version: "1.6.0"
```

### Internal Remote State Approach (Default)

The default approach uses Cloud Posse's remote state module to fetch EKS cluster information:

```yaml
components:
  terraform:
    eks/karpenter:
      vars:
        enabled: true
        # account_map_enabled defaults to true
        eks_component_name: "eks/cluster-blue"
```

## Troubleshooting

For Karpenter issues, checkout the [Karpenter Troubleshooting Guide](https://karpenter.sh/docs/troubleshooting/)


<!-- markdownlint-disable -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3.0 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 4.9.0, < 6.0.0 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | >= 2.0.0, < 3.0.0 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | >= 2.7.1, != 2.21.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 4.9.0, < 6.0.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_eks"></a> [eks](#module\_eks) | cloudposse/stack-config/yaml//modules/remote-state | 1.8.0 |
| <a name="module_iam_roles"></a> [iam\_roles](#module\_iam\_roles) | ../../account-map/modules/iam-roles | n/a |
| <a name="module_karpenter"></a> [karpenter](#module\_karpenter) | cloudposse/helm-release/aws | 0.10.1 |
| <a name="module_karpenter_crd"></a> [karpenter\_crd](#module\_karpenter\_crd) | cloudposse/helm-release/aws | 0.10.1 |
| <a name="module_this"></a> [this](#module\_this) | cloudposse/label/null | 0.25.0 |

## Resources

| Name | Type |
|------|------|
| [aws_cloudwatch_event_rule.interruption_handler](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_event_rule) | resource |
| [aws_cloudwatch_event_target.interruption_handler](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_event_target) | resource |
| [aws_iam_policy.v1alpha](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_policy) | resource |
| [aws_iam_role_policy_attachment.v1alpha](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_sqs_queue.interruption_handler](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue) | resource |
| [aws_sqs_queue_policy.interruption_handler](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue_policy) | resource |
| [aws_eks_cluster_auth.eks](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/eks_cluster_auth) | data source |
| [aws_iam_policy_document.interruption_handler](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_partition.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/partition) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_account_map_enabled"></a> [account\_map\_enabled](#input\_account\_map\_enabled) | Enable the account map component lookup. When disabled, use the `eks` variable to provide static EKS cluster configuration. | `bool` | `true` | no |
| <a name="input_additional_settings"></a> [additional\_settings](#input\_additional\_settings) | Additional settings to merge into the Karpenter controller settings.<br/>This is useful for setting featureGates or other advanced settings that may<br/>vary by chart version. These settings will be merged with the base settings<br/>and take precedence over any conflicting keys.<br/><br/>Example:<br/>additional\_settings = {<br/>  featureGates = {<br/>    nodeRepair = false<br/>    reservedCapacity = true<br/>    spotToSpotConsolidation = false<br/>  }<br/>} | `any` | `{}` | no |
| <a name="input_additional_tag_map"></a> [additional\_tag\_map](#input\_additional\_tag\_map) | Additional key-value pairs to add to each map in `tags_as_list_of_maps`. Not added to `tags` or `id`.<br/>This is for some rare cases where resources want additional configuration of tags<br/>and therefore take a list of maps with tag key, value, and additional configuration. | `map(string)` | `{}` | no |
| <a name="input_atomic"></a> [atomic](#input\_atomic) | If set, installation process purges chart on fail. The wait flag will be set automatically if atomic is used | `bool` | `true` | no |
| <a name="input_attributes"></a> [attributes](#input\_attributes) | ID element. Additional attributes (e.g. `workers` or `cluster`) to add to `id`,<br/>in the order they appear in the list. New attributes are appended to the<br/>end of the list. The elements of the list are joined by the `delimiter`<br/>and treated as a single ID element. | `list(string)` | `[]` | no |
| <a name="input_chart"></a> [chart](#input\_chart) | Chart name to be installed. The chart name can be local path, a URL to a chart, or the name of the chart if `repository` is specified. It is also possible to use the `<repository>/<chart>` format here if you are running Terraform on a system that the repository has been added to with `helm repo add` but this is not recommended | `string` | n/a | yes |
| <a name="input_chart_description"></a> [chart\_description](#input\_chart\_description) | Set release description attribute (visible in the history) | `string` | `null` | no |
| <a name="input_chart_repository"></a> [chart\_repository](#input\_chart\_repository) | Repository URL where to locate the requested chart | `string` | n/a | yes |
| <a name="input_chart_values"></a> [chart\_values](#input\_chart\_values) | Additional values to yamlencode as `helm_release` values | `any` | `{}` | no |
| <a name="input_chart_version"></a> [chart\_version](#input\_chart\_version) | Specify the exact chart version to install. If this is not specified, the latest version is installed | `string` | `null` | no |
| <a name="input_cleanup_on_fail"></a> [cleanup\_on\_fail](#input\_cleanup\_on\_fail) | Allow deletion of new resources created in this upgrade when upgrade fails | `bool` | `true` | no |
| <a name="input_context"></a> [context](#input\_context) | Single object for setting entire context at once.<br/>See description of individual variables for details.<br/>Leave string and numeric variables as `null` to use default value.<br/>Individual variable settings (non-null) override settings in context object,<br/>except for attributes, tags, and additional\_tag\_map, which are merged. | `any` | <pre>{<br/>  "additional_tag_map": {},<br/>  "attributes": [],<br/>  "delimiter": null,<br/>  "descriptor_formats": {},<br/>  "enabled": true,<br/>  "environment": null,<br/>  "id_length_limit": null,<br/>  "label_key_case": null,<br/>  "label_order": [],<br/>  "label_value_case": null,<br/>  "labels_as_tags": [<br/>    "unset"<br/>  ],<br/>  "name": null,<br/>  "namespace": null,<br/>  "regex_replace_chars": null,<br/>  "stage": null,<br/>  "tags": {},<br/>  "tenant": null<br/>}</pre> | no |
| <a name="input_crd_chart"></a> [crd\_chart](#input\_crd\_chart) | The name of the Karpenter CRD chart to be installed, if `var.crd_chart_enabled` is set to `true`. | `string` | `"karpenter-crd"` | no |
| <a name="input_crd_chart_enabled"></a> [crd\_chart\_enabled](#input\_crd\_chart\_enabled) | `karpenter-crd` can be installed as an independent helm chart to manage the lifecycle of Karpenter CRDs. Set to `true` to install this CRD helm chart before the primary karpenter chart. | `bool` | `false` | no |
| <a name="input_delimiter"></a> [delimiter](#input\_delimiter) | Delimiter to be used between ID elements.<br/>Defaults to `-` (hyphen). Set to `""` to use no delimiter at all. | `string` | `null` | no |
| <a name="input_descriptor_formats"></a> [descriptor\_formats](#input\_descriptor\_formats) | Describe additional descriptors to be output in the `descriptors` output map.<br/>Map of maps. Keys are names of descriptors. Values are maps of the form<br/>`{<br/>  format = string<br/>  labels = list(string)<br/>}`<br/>(Type is `any` so the map values can later be enhanced to provide additional options.)<br/>`format` is a Terraform format string to be passed to the `format()` function.<br/>`labels` is a list of labels, in order, to pass to `format()` function.<br/>Label values will be normalized before being passed to `format()` so they will be<br/>identical to how they appear in `id`.<br/>Default is `{}` (`descriptors` output will be empty). | `any` | `{}` | no |
| <a name="input_eks"></a> [eks](#input\_eks) | EKS cluster configuration. Required when `account_map_enabled` is `false`. | <pre>object({<br/>    eks_cluster_id                         = optional(string, "")<br/>    eks_cluster_arn                        = optional(string, "")<br/>    eks_cluster_endpoint                   = optional(string, "")<br/>    eks_cluster_certificate_authority_data = optional(string, "")<br/>    eks_cluster_identity_oidc_issuer       = optional(string, "")<br/>    karpenter_iam_role_arn                 = optional(string, "")<br/>  })</pre> | `{}` | no |
| <a name="input_eks_component_name"></a> [eks\_component\_name](#input\_eks\_component\_name) | The name of the eks component. Used when `account_map_enabled` is `true`. | `string` | `"eks/cluster"` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `null` | no |
| <a name="input_environment"></a> [environment](#input\_environment) | ID element. Usually used for region e.g. 'uw2', 'us-west-2', OR role 'prod', 'staging', 'dev', 'UAT' | `string` | `null` | no |
| <a name="input_helm_manifest_experiment_enabled"></a> [helm\_manifest\_experiment\_enabled](#input\_helm\_manifest\_experiment\_enabled) | Enable storing of the rendered manifest for helm\_release so the full diff of what is changing can been seen in the plan | `bool` | `false` | no |
| <a name="input_id_length_limit"></a> [id\_length\_limit](#input\_id\_length\_limit) | Limit `id` to this many characters (minimum 6).<br/>Set to `0` for unlimited length.<br/>Set to `null` for keep the existing setting, which defaults to `0`.<br/>Does not affect `id_full`. | `number` | `null` | no |
| <a name="input_interruption_handler_enabled"></a> [interruption\_handler\_enabled](#input\_interruption\_handler\_enabled) | If `true`, deploy a SQS queue and Event Bridge rules to enable interruption handling by Karpenter.<br/>  https://karpenter.sh/docs/concepts/disruption/#interruption | `bool` | `true` | no |
| <a name="input_interruption_queue_message_retention"></a> [interruption\_queue\_message\_retention](#input\_interruption\_queue\_message\_retention) | The message retention in seconds for the interruption handler SQS queue. | `number` | `300` | no |
| <a name="input_kube_data_auth_enabled"></a> [kube\_data\_auth\_enabled](#input\_kube\_data\_auth\_enabled) | If `true`, use an `aws_eks_cluster_auth` data source to authenticate to the EKS cluster.<br/>Disabled by `kubeconfig_file_enabled` or `kube_exec_auth_enabled`. | `bool` | `false` | no |
| <a name="input_kube_exec_auth_aws_profile"></a> [kube\_exec\_auth\_aws\_profile](#input\_kube\_exec\_auth\_aws\_profile) | The AWS config profile for `aws eks get-token` to use | `string` | `""` | no |
| <a name="input_kube_exec_auth_aws_profile_enabled"></a> [kube\_exec\_auth\_aws\_profile\_enabled](#input\_kube\_exec\_auth\_aws\_profile\_enabled) | If `true`, pass `kube_exec_auth_aws_profile` as the `profile` to `aws eks get-token` | `bool` | `false` | no |
| <a name="input_kube_exec_auth_enabled"></a> [kube\_exec\_auth\_enabled](#input\_kube\_exec\_auth\_enabled) | If `true`, use the Kubernetes provider `exec` feature to execute `aws eks get-token` to authenticate to the EKS cluster.<br/>Disabled by `kubeconfig_file_enabled`, overrides `kube_data_auth_enabled`. | `bool` | `true` | no |
| <a name="input_kube_exec_auth_role_arn"></a> [kube\_exec\_auth\_role\_arn](#input\_kube\_exec\_auth\_role\_arn) | The role ARN for `aws eks get-token` to use | `string` | `""` | no |
| <a name="input_kube_exec_auth_role_arn_enabled"></a> [kube\_exec\_auth\_role\_arn\_enabled](#input\_kube\_exec\_auth\_role\_arn\_enabled) | If `true`, pass `kube_exec_auth_role_arn` as the role ARN to `aws eks get-token` | `bool` | `true` | no |
| <a name="input_kubeconfig_context"></a> [kubeconfig\_context](#input\_kubeconfig\_context) | Context to choose from the Kubernetes config file.<br/>If supplied, `kubeconfig_context_format` will be ignored. | `string` | `""` | no |
| <a name="input_kubeconfig_context_format"></a> [kubeconfig\_context\_format](#input\_kubeconfig\_context\_format) | A format string to use for creating the `kubectl` context name when<br/>`kubeconfig_file_enabled` is `true` and `kubeconfig_context` is not supplied.<br/>Must include a single `%s` which will be replaced with the cluster name. | `string` | `""` | no |
| <a name="input_kubeconfig_exec_auth_api_version"></a> [kubeconfig\_exec\_auth\_api\_version](#input\_kubeconfig\_exec\_auth\_api\_version) | The Kubernetes API version of the credentials returned by the `exec` auth plugin | `string` | `"client.authentication.k8s.io/v1beta1"` | no |
| <a name="input_kubeconfig_file"></a> [kubeconfig\_file](#input\_kubeconfig\_file) | The Kubernetes provider `config_path` setting to use when `kubeconfig_file_enabled` is `true` | `string` | `""` | no |
| <a name="input_kubeconfig_file_enabled"></a> [kubeconfig\_file\_enabled](#input\_kubeconfig\_file\_enabled) | If `true`, configure the Kubernetes provider with `kubeconfig_file` and use that kubeconfig file for authenticating to the EKS cluster | `bool` | `false` | no |
| <a name="input_label_key_case"></a> [label\_key\_case](#input\_label\_key\_case) | Controls the letter case of the `tags` keys (label names) for tags generated by this module.<br/>Does not affect keys of tags passed in via the `tags` input.<br/>Possible values: `lower`, `title`, `upper`.<br/>Default value: `title`. | `string` | `null` | no |
| <a name="input_label_order"></a> [label\_order](#input\_label\_order) | The order in which the labels (ID elements) appear in the `id`.<br/>Defaults to ["namespace", "environment", "stage", "name", "attributes"].<br/>You can omit any of the 6 labels ("tenant" is the 6th), but at least one must be present. | `list(string)` | `null` | no |
| <a name="input_label_value_case"></a> [label\_value\_case](#input\_label\_value\_case) | Controls the letter case of ID elements (labels) as included in `id`,<br/>set as tag values, and output by this module individually.<br/>Does not affect values of tags passed in via the `tags` input.<br/>Possible values: `lower`, `title`, `upper` and `none` (no transformation).<br/>Set this to `title` and set `delimiter` to `""` to yield Pascal Case IDs.<br/>Default value: `lower`. | `string` | `null` | no |
| <a name="input_labels_as_tags"></a> [labels\_as\_tags](#input\_labels\_as\_tags) | Set of labels (ID elements) to include as tags in the `tags` output.<br/>Default is to include all labels.<br/>Tags with empty values will not be included in the `tags` output.<br/>Set to `[]` to suppress all generated tags.<br/>**Notes:**<br/>  The value of the `name` tag, if included, will be the `id`, not the `name`.<br/>  Unlike other `null-label` inputs, the initial setting of `labels_as_tags` cannot be<br/>  changed in later chained modules. Attempts to change it will be silently ignored. | `set(string)` | <pre>[<br/>  "default"<br/>]</pre> | no |
| <a name="input_logging"></a> [logging](#input\_logging) | A subset of the logging settings for the Karpenter controller | <pre>object({<br/>    enabled = optional(bool, true)<br/>    level = optional(object({<br/>      controller = optional(string, "info")<br/>      global     = optional(string, "info")<br/>      webhook    = optional(string, "error")<br/>    }), {})<br/>  })</pre> | `{}` | no |
| <a name="input_metrics_enabled"></a> [metrics\_enabled](#input\_metrics\_enabled) | Whether to expose the Karpenter's Prometheus metric | `bool` | `true` | no |
| <a name="input_metrics_port"></a> [metrics\_port](#input\_metrics\_port) | Container port to use for metrics | `number` | `8080` | no |
| <a name="input_name"></a> [name](#input\_name) | ID element. Usually the component or solution name, e.g. 'app' or 'jenkins'.<br/>This is the only ID element not also included as a `tag`.<br/>The "name" tag is set to the full `id` string. There is no tag with the value of the `name` input. | `string` | `null` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | ID element. Usually an abbreviation of your organization name, e.g. 'eg' or 'cp', to help ensure generated IDs are globally unique | `string` | `null` | no |
| <a name="input_regex_replace_chars"></a> [regex\_replace\_chars](#input\_regex\_replace\_chars) | Terraform regular expression (regex) string.<br/>Characters matching the regex will be removed from the ID elements.<br/>If not set, `"/[^a-zA-Z0-9-]/"` is used to remove all characters other than hyphens, letters and digits. | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | AWS Region | `string` | n/a | yes |
| <a name="input_replicas"></a> [replicas](#input\_replicas) | The number of Karpenter controller replicas to run | `number` | `2` | no |
| <a name="input_resources"></a> [resources](#input\_resources) | The CPU and memory of the deployment's limits and requests | <pre>object({<br/>    limits = object({<br/>      cpu    = string<br/>      memory = string<br/>    })<br/>    requests = object({<br/>      cpu    = string<br/>      memory = string<br/>    })<br/>  })</pre> | n/a | yes |
| <a name="input_settings"></a> [settings](#input\_settings) | A subset of the settings for the Karpenter controller.<br/>Some settings are implicitly set by this component, such as `clusterName` and<br/>`interruptionQueue`. All settings can be overridden by providing a `settings`<br/>section in the `chart_values` variable. The settings provided here are the ones<br/>mostly likely to be set to other than default values, and are provided here for convenience. | <pre>object({<br/>    batch_idle_duration = optional(string, "1s")<br/>    batch_max_duration  = optional(string, "10s")<br/>  })</pre> | `{}` | no |
| <a name="input_stage"></a> [stage](#input\_stage) | ID element. Usually used to indicate role, e.g. 'prod', 'staging', 'source', 'build', 'test', 'deploy', 'release' | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Additional tags (e.g. `{'BusinessUnit': 'XYZ'}`).<br/>Neither the tag keys nor the tag values will be modified by this module. | `map(string)` | `{}` | no |
| <a name="input_tenant"></a> [tenant](#input\_tenant) | ID element \_(Rarely used, not included by default)\_. A customer identifier, indicating who this instance of a resource is for | `string` | `null` | no |
| <a name="input_timeout"></a> [timeout](#input\_timeout) | Time in seconds to wait for any individual kubernetes operation (like Jobs for hooks). Defaults to `300` seconds | `number` | `null` | no |
| <a name="input_wait"></a> [wait](#input\_wait) | Will wait until all resources are in a ready state before marking the release as successful. It will wait for as long as `timeout`. Defaults to `true` | `bool` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_metadata"></a> [metadata](#output\_metadata) | Block status of the deployed release |
<!-- markdownlint-restore -->



## References


- [Karpenter Getting Started: Create NodePool](https://karpenter.sh/v0.36/getting-started/getting-started-with-karpenter/#5-create-nodepool) -

- [Karpenter Concepts: Interruption](https://karpenter.sh/v0.36/concepts/disruption/#interruption) -

- [Karpenter Concepts: Taints](https://karpenter.sh/v0.36/concepts/nodepools/#taints) -

- [Karpenter Concepts: Requirements](https://karpenter.sh/v0.36/concepts/nodepools/#spectemplatespecrequirements) -

- [Karpenter Getting Started](https://karpenter.sh/v0.36/getting-started/getting-started-with-karpenter/) -

- [AWS EKS Best Practices: Karpenter](https://aws.github.io/aws-eks-best-practices/karpenter) -

- [Karpenter](https://karpenter.sh) -

- [AWS Blog: Introducing Karpenter](https://aws.amazon.com/blogs/aws/introducing-karpenter-an-open-source-high-performance-kubernetes-cluster-autoscaler) -

- [aws/karpenter](https://github.com/aws/karpenter) -

- [EC2 Spot Workshops: Karpenter](https://ec2spotworkshops.com/karpenter.html) -

- [EKS Workshop: Karpenter](https://www.eksworkshop.com/docs/autoscaling/compute/karpenter/) -

- [EKS Pod Execution Role](https://docs.aws.amazon.com/eks/latest/userguide/pod-execution-role.html) -

- [AWS KB: Fargate troubleshoot profile creation](https://aws.amazon.com/premiumsupport/knowledge-center/fargate-troubleshoot-profile-creation) -

- [HashiCorp Learn: Kubernetes CRD](https://learn.hashicorp.com/tutorials/terraform/kubernetes-crd-faas) -

- [AWS Batch: Spot Fleet IAM role](https://docs.aws.amazon.com/batch/latest/userguide/spot_fleet_IAM_role.html) -

- [AWS IAM: Service-linked roles](https://docs.aws.amazon.com/IAM/latest/UserGuide/using-service-linked-roles.html) -

- [Karpenter Troubleshooting Guide](https://karpenter.sh/docs/troubleshooting/) -

- [Getting started with Terraform (EKS Blueprints)](https://aws-ia.github.io/terraform-aws-eks-blueprints/getting-started/) -

- [Getting started with eksctl for Karpenter](https://karpenter.sh/docs/getting-started/getting-started-with-karpenter/) -




[<img src="https://cloudposse.com/logo-300x69.svg" height="32" align="right"/>](https://cpco.io/homepage?utm_source=github&utm_medium=readme&utm_campaign=cloudposse-terraform-components/aws-eks-karpenter-controller&utm_content=)
