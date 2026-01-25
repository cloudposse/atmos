---
tags:
  - component/eks/karpenter-node-pool
  - layer/eks
  - provider/aws
  - provider/helm
---

# Component: `eks-karpenter-node-pool`

This component deploys [Karpenter NodePools](https://karpenter.sh/docs/concepts/nodepools/) to an EKS cluster.

Karpenter is still rapidly evolving. At this time, this component only supports a subset of the features
available in Karpenter. Support could be added for additional features as needed.

Not supported:

- Elements of NodePool:
  - [`template.spec.kubelet`](https://karpenter.sh/docs/concepts/nodepools/#spectemplatespeckubelet)
- Elements of NodeClass:
  - `subnetSelectorTerms`. This component only supports selecting all public or all private subnets of the referenced
    EKS cluster.
  - `securityGroupSelectorTerms`. This component only supports selecting the security group of the referenced EKS
    cluster.
  - `amiSelectorTerms`. Such terms override the `amiFamily` setting, which is the only AMI selection supported by this
    component.
  - `instanceStorePolicy`
  - `associatePublicIPAddress`
## Usage

**Stack Level**: Regional

If provisioning more than one NodePool, it is
[best practice](https://aws.github.io/aws-eks-best-practices/karpenter/#creating-nodepools) to create NodePools that are
mutually exclusive or weighted.

## Configuration Approaches

This component supports three configuration approaches controlled by the `account_map_enabled` variable.

### Option 1: Direct Input Variables (`account_map_enabled: false`)

Set `account_map_enabled: false` and provide the required values via the `eks` and `vpc` object variables.
This approach is simpler and avoids cross-component dependencies.

Example using direct inputs:

```yaml
components:
  terraform:
    eks/karpenter-node-pool:
      vars:
        enabled: true
        account_map_enabled: false
        name: "karpenter-node-pool"
        eks:
          eks_cluster_id: "my-cluster"
          eks_cluster_endpoint: "https://XXXXXXXX.gr7.us-west-2.eks.amazonaws.com"
          eks_cluster_certificate_authority_data: "LS0tLS1CRUdJTi..."
          karpenter_iam_role_name: "my-cluster-karpenter"
        vpc:
          private_subnet_ids:
            - "subnet-xxxxxxxxx"
            - "subnet-yyyyyyyyy"
        # ... node_pools configuration
```

### Option 2: Using Atmos `!terraform.state` (Recommended)

For Atmos users, the recommended approach is to use `!terraform.state` to dynamically fetch values from
other component outputs and pass them as direct input variables. This keeps dependencies explicit in your
stack configuration without using internal remote-state modules.

Example using Atmos `!terraform.state`:

```yaml
components:
  terraform:
    eks/karpenter-node-pool:
      vars:
        enabled: true
        account_map_enabled: false
        name: "karpenter-node-pool"
        eks:
          eks_cluster_id: !terraform.state eks/cluster eks_cluster_id
          eks_cluster_endpoint: !terraform.state eks/cluster eks_cluster_endpoint
          eks_cluster_certificate_authority_data: !terraform.state eks/cluster eks_cluster_certificate_authority_data
          karpenter_iam_role_name: !terraform.state eks/cluster karpenter_iam_role_name
        vpc:
          private_subnet_ids: !terraform.state vpc private_subnet_ids
        node_pools:
          default:
            name: default
            private_subnets_enabled: true
            # ... rest of node pool configuration
```

This approach:
- Uses native Atmos functionality for cross-component references
- Makes dependencies explicit and visible in stack configuration
- Does not use internal remote-state modules (cleaner component code)
- Supports referencing components in different stacks with extended syntax

For referencing components in different stacks:
```yaml
eks:
  eks_cluster_id: !terraform.state eks/cluster <stack> eks_cluster_id
```

### Option 3: Internal Remote State Modules (`account_map_enabled: true`, default, deprecated)

> **Warning:** The `account_map_enabled: true` setting and `eks_component_name`/`vpc_component_name` variables
> are deprecated and will be removed in a future version. Please migrate to using `!terraform.state` (Option 2)
> or direct input variables (Option 1).

When `account_map_enabled` is `true` (the default), the component uses internal CloudPosse remote-state modules
to fetch EKS cluster and VPC information. This approach is being phased out in favor of explicit variable passing
via `!terraform.state` which provides better visibility into component dependencies.

Example using CloudPosse remote state:

```yaml
components:
  terraform:
    eks/karpenter-node-pool:
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        enabled: true
        account_map_enabled: true  # default, can be omitted
        eks_component_name: eks/cluster
        vpc_component_name: vpc
        name: "karpenter-node-pool"
        # https://karpenter.sh/v0.36.0/docs/concepts/nodepools/
        node_pools:
          default:
            name: default
            # Whether to place EC2 instances launched by Karpenter into VPC private subnets. Set it to `false` to use public subnets
            private_subnets_enabled: true
            disruption:
              consolidation_policy: WhenUnderutilized
              consolidate_after: 1h
              max_instance_lifetime: 336h
              budgets:
                # This budget allows 0 disruptions during business hours (from 9am to 5pm) on weekdays
                - schedule: "0 9 * * mon-fri"
                  duration: 8h
                  nodes: "0"
            # The total cpu of the cluster. Maps to spec.limits.cpu in the Karpenter NodeClass
            total_cpu_limit: "100"
            # The total memory of the cluster. Maps to spec.limits.memory in the Karpenter NodeClass
            total_memory_limit: "1000Gi"
            # The total GPU of the cluster. Maps to spec.limits for GPU in the Karpenter NodeClass
            gpu_total_limits:
              "nvidia.com/gpu" = "1"
            # The weight of the node pool. See https://karpenter.sh/docs/concepts/scheduling/#weighted-nodepools
            weight: 50
            # Taints to apply to the nodes in the node pool. See https://karpenter.sh/docs/concepts/nodeclasses/#spectaints
            taints:
              - key: "node.kubernetes.io/unreachable"
                effect: "NoExecute"
                value: "true"
            # Taints to apply to the nodes in the node pool at startup. See https://karpenter.sh/docs/concepts/nodeclasses/#specstartuptaints
            startup_taints:
              - key: "node.kubernetes.io/unreachable"
                effect: "NoExecute"
                value: "true"
            # Metadata options for the node pool. See https://karpenter.sh/docs/concepts/nodeclasses/#specmetadataoptions
            metadata_options:
              httpEndpoint: "enabled" # allows the node to call the AWS metadata service
              httpProtocolIPv6: "disabled"
              httpPutResponseHopLimit: 2
              httpTokens: "required"
            # The AMI used by Karpenter provisioner when provisioning nodes. Based on the value set for amiFamily, Karpenter will automatically query for the appropriate EKS optimized AMI via AWS Systems Manager (SSM)
            # Bottlerocket, AL2, Ubuntu
            # https://karpenter.sh/v0.18.0/aws/provisioning/#amazon-machine-image-ami-family
            ami_family: AL2
            # Karpenter provisioner block device mappings.
            block_device_mappings:
              - deviceName: /dev/xvda
                ebs:
                  volumeSize: 200Gi
                  volumeType: gp3
                  encrypted: true
                  deleteOnTermination: true
            # Set acceptable (In) and unacceptable (Out) Kubernetes and Karpenter values for node provisioning based on
            # Well-Known Labels and cloud-specific settings. These can include instance types, zones, computer architecture,
            # and capacity type (such as AWS spot or on-demand).
            # See https://karpenter.sh/v0.18.0/provisioner/#specrequirements for more details
            requirements:
              - key: "karpenter.sh/capacity-type"
                operator: "In"
                values:
                  - "on-demand"
                  - "spot"
              - key: "node.kubernetes.io/instance-type"
                operator: "In"
                # See https://aws.amazon.com/ec2/instance-explorer/ and https://aws.amazon.com/ec2/instance-types/
                # Values limited by DenyEC2InstancesWithoutEncryptionInTransit service control policy
                # See https://github.com/cloudposse/terraform-aws-service-control-policies/blob/master/catalog/ec2-policies.yaml
                # Karpenter recommends allowing at least 20 instance types to ensure availability.
                values:
                  - "c5n.2xlarge"
                  - "c5n.xlarge"
                  - "c5n.large"
                  - "c6i.2xlarge"
                  - "c6i.xlarge"
                  - "c6i.large"
                  - "m5n.2xlarge"
                  - "m5n.xlarge"
                  - "m5n.large"
                  - "m5zn.2xlarge"
                  - "m5zn.xlarge"
                  - "m5zn.large"
                  - "m6i.2xlarge"
                  - "m6i.xlarge"
                  - "m6i.large"
                  - "r5n.2xlarge"
                  - "r5n.xlarge"
                  - "r5n.large"
                  - "r6i.2xlarge"
                  - "r6i.xlarge"
                  - "r6i.large"
              - key: "kubernetes.io/arch"
                operator: "In"
                values:
                  - "amd64"
```


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
| <a name="provider_kubernetes"></a> [kubernetes](#provider\_kubernetes) | >= 2.7.1, != 2.21.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_eks"></a> [eks](#module\_eks) | cloudposse/stack-config/yaml//modules/remote-state | 1.8.0 |
| <a name="module_iam_roles"></a> [iam\_roles](#module\_iam\_roles) | ../../account-map/modules/iam-roles | n/a |
| <a name="module_this"></a> [this](#module\_this) | cloudposse/label/null | 0.25.0 |
| <a name="module_vpc"></a> [vpc](#module\_vpc) | cloudposse/stack-config/yaml//modules/remote-state | 1.8.0 |

## Resources

| Name | Type |
|------|------|
| [kubernetes_manifest.ec2_node_class](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/manifest) | resource |
| [kubernetes_manifest.node_pool](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/manifest) | resource |
| [aws_eks_cluster_auth.eks](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/eks_cluster_auth) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_account_map_enabled"></a> [account\_map\_enabled](#input\_account\_map\_enabled) | Enable account map and remote state lookups.<br/>When `true`, fetch EKS cluster and VPC information from Terraform remote state.<br/>When `false`, use the `eks` and `vpc` variables to provide values directly. | `bool` | `true` | no |
| <a name="input_additional_tag_map"></a> [additional\_tag\_map](#input\_additional\_tag\_map) | Additional key-value pairs to add to each map in `tags_as_list_of_maps`. Not added to `tags` or `id`.<br/>This is for some rare cases where resources want additional configuration of tags<br/>and therefore take a list of maps with tag key, value, and additional configuration. | `map(string)` | `{}` | no |
| <a name="input_attributes"></a> [attributes](#input\_attributes) | ID element. Additional attributes (e.g. `workers` or `cluster`) to add to `id`,<br/>in the order they appear in the list. New attributes are appended to the<br/>end of the list. The elements of the list are joined by the `delimiter`<br/>and treated as a single ID element. | `list(string)` | `[]` | no |
| <a name="input_context"></a> [context](#input\_context) | Single object for setting entire context at once.<br/>See description of individual variables for details.<br/>Leave string and numeric variables as `null` to use default value.<br/>Individual variable settings (non-null) override settings in context object,<br/>except for attributes, tags, and additional\_tag\_map, which are merged. | `any` | <pre>{<br/>  "additional_tag_map": {},<br/>  "attributes": [],<br/>  "delimiter": null,<br/>  "descriptor_formats": {},<br/>  "enabled": true,<br/>  "environment": null,<br/>  "id_length_limit": null,<br/>  "label_key_case": null,<br/>  "label_order": [],<br/>  "label_value_case": null,<br/>  "labels_as_tags": [<br/>    "unset"<br/>  ],<br/>  "name": null,<br/>  "namespace": null,<br/>  "regex_replace_chars": null,<br/>  "stage": null,<br/>  "tags": {},<br/>  "tenant": null<br/>}</pre> | no |
| <a name="input_delimiter"></a> [delimiter](#input\_delimiter) | Delimiter to be used between ID elements.<br/>Defaults to `-` (hyphen). Set to `""` to use no delimiter at all. | `string` | `null` | no |
| <a name="input_descriptor_formats"></a> [descriptor\_formats](#input\_descriptor\_formats) | Describe additional descriptors to be output in the `descriptors` output map.<br/>Map of maps. Keys are names of descriptors. Values are maps of the form<br/>`{<br/>   format = string<br/>   labels = list(string)<br/>}`<br/>(Type is `any` so the map values can later be enhanced to provide additional options.)<br/>`format` is a Terraform format string to be passed to the `format()` function.<br/>`labels` is a list of labels, in order, to pass to `format()` function.<br/>Label values will be normalized before being passed to `format()` so they will be<br/>identical to how they appear in `id`.<br/>Default is `{}` (`descriptors` output will be empty). | `any` | `{}` | no |
| <a name="input_eks"></a> [eks](#input\_eks) | EKS cluster configuration to use when `account_map_enabled` is `false`.<br/>Provides cluster details for Karpenter node pool configuration. | <pre>object({<br/>    eks_cluster_id                         = optional(string, "")<br/>    eks_cluster_arn                        = optional(string, "")<br/>    eks_cluster_endpoint                   = optional(string, "")<br/>    eks_cluster_certificate_authority_data = optional(string, "")<br/>    eks_cluster_identity_oidc_issuer       = optional(string, "")<br/>    karpenter_iam_role_name                = optional(string, "")<br/>    karpenter_node_role_arn                = optional(string, "")<br/>  })</pre> | <pre>{<br/>  "eks_cluster_arn": "",<br/>  "eks_cluster_certificate_authority_data": "",<br/>  "eks_cluster_endpoint": "",<br/>  "eks_cluster_id": "",<br/>  "eks_cluster_identity_oidc_issuer": "",<br/>  "karpenter_iam_role_name": "",<br/>  "karpenter_node_role_arn": ""<br/>}</pre> | no |
| <a name="input_eks_component_name"></a> [eks\_component\_name](#input\_eks\_component\_name) | The name of the EKS component. Used to fetch EKS cluster information from remote state<br/>when `account_map_enabled` is `true`.<br/><br/>DEPRECATED: This variable (along with account\_map\_enabled=true) is deprecated and<br/>will be removed in a future version. Set `account_map_enabled = false` and use<br/>the direct EKS cluster input variables instead. | `string` | `"eks/cluster"` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `null` | no |
| <a name="input_environment"></a> [environment](#input\_environment) | ID element. Usually used for region e.g. 'uw2', 'us-west-2', OR role 'prod', 'staging', 'dev', 'UAT' | `string` | `null` | no |
| <a name="input_helm_manifest_experiment_enabled"></a> [helm\_manifest\_experiment\_enabled](#input\_helm\_manifest\_experiment\_enabled) | Enable storing of the rendered manifest for helm\_release so the full diff of what is changing can been seen in the plan | `bool` | `false` | no |
| <a name="input_id_length_limit"></a> [id\_length\_limit](#input\_id\_length\_limit) | Limit `id` to this many characters (minimum 6).<br/>Set to `0` for unlimited length.<br/>Set to `null` for keep the existing setting, which defaults to `0`.<br/>Does not affect `id_full`. | `number` | `null` | no |
| <a name="input_import_profile_name"></a> [import\_profile\_name](#input\_import\_profile\_name) | AWS Profile name to use when importing a resource | `string` | `null` | no |
| <a name="input_import_role_arn"></a> [import\_role\_arn](#input\_import\_role\_arn) | IAM Role ARN to use when importing a resource | `string` | `null` | no |
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
| <a name="input_name"></a> [name](#input\_name) | ID element. Usually the component or solution name, e.g. 'app' or 'jenkins'.<br/>This is the only ID element not also included as a `tag`.<br/>The "name" tag is set to the full `id` string. There is no tag with the value of the `name` input. | `string` | `null` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | ID element. Usually an abbreviation of your organization name, e.g. 'eg' or 'cp', to help ensure generated IDs are globally unique | `string` | `null` | no |
| <a name="input_node_pools"></a> [node\_pools](#input\_node\_pools) | Configuration for node pools. See code for details. | <pre>map(object({<br/>    # The name of the Karpenter provisioner. The map key is used if this is not set.<br/>    name = optional(string)<br/>    # Whether to place EC2 instances launched by Karpenter into VPC private subnets. Set it to `false` to use public subnets.<br/>    private_subnets_enabled = bool<br/>    # The Disruption spec controls how Karpenter scales down the node group.<br/>    # See the example (sadly not the specific `spec.disruption` documentation) at https://karpenter.sh/docs/concepts/nodepools/ for details<br/>    disruption = optional(object({<br/>      # Describes which types of Nodes Karpenter should consider for consolidation.<br/>      # If using 'WhenUnderutilized', Karpenter will consider all nodes for consolidation and attempt to remove or<br/>      # replace Nodes when it discovers that the Node is underutilized and could be changed to reduce cost.<br/>      # If using `WhenEmpty`, Karpenter will only consider nodes for consolidation that contain no workload pods.<br/>      consolidation_policy = optional(string, "WhenUnderutilized")<br/><br/>      # The amount of time Karpenter should wait after discovering a consolidation decision (`go` duration string, s, m, or h).<br/>      # This value can currently (v0.36.0) only be set when the consolidationPolicy is 'WhenEmpty'.<br/>      # You can choose to disable consolidation entirely by setting the string value 'Never' here.<br/>      # Earlier versions of Karpenter called this field `ttl_seconds_after_empty`.<br/>      consolidate_after = optional(string)<br/><br/>      # The amount of time a Node can live on the cluster before being removed (`go` duration string, s, m, or h).<br/>      # You can choose to disable expiration entirely by setting the string value 'Never' here.<br/>      # This module sets a default of 336 hours (14 days), while the Karpenter default is 720 hours (30 days).<br/>      # Note that Karpenter calls this field "expiresAfter", and earlier versions called it `ttl_seconds_until_expired`,<br/>      # but we call it "max_instance_lifetime" to match the corresponding field in EC2 Auto Scaling Groups.<br/>      max_instance_lifetime = optional(string, "336h")<br/><br/>      # Budgets control the the maximum number of NodeClaims owned by this NodePool that can be terminating at once.<br/>      # See https://karpenter.sh/docs/concepts/disruption/#disruption-budgets for details.<br/>      # A percentage is the percentage of the total number of active, ready nodes not being deleted, rounded up.<br/>      # If there are multiple active budgets, Karpenter uses the most restrictive value.<br/>      # If left undefined, this will default to one budget with a value of nodes: 10%.<br/>      # Note that budgets do not prevent or limit involuntary terminations.<br/>      # Example:<br/>      #   On Weekdays during business hours, don't do any deprovisioning.<br/>      #     budgets = {<br/>      #       schedule = "0 9 * * mon-fri"<br/>      #       duration = 8h<br/>      #       nodes    = "0"<br/>      #     }<br/>      budgets = optional(list(object({<br/>        # The schedule specifies when a budget begins being active, using extended cronjob syntax.<br/>        # See https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/#schedule-syntax for syntax details.<br/>        # Timezones are not supported. This field is required if Duration is set.<br/>        schedule = optional(string)<br/>        # Duration determines how long a Budget is active after each Scheduled start.<br/>        # If omitted, the budget is always active. This is required if Schedule is set.<br/>        # Must be a whole number of minutes and hours, as cron does not work in seconds,<br/>        # but since Go's `duration.String()` always adds a "0s" at the end, that is allowed.<br/>        duration = optional(string)<br/>        # The percentage or number of nodes that Karpenter can scale down during the budget.<br/>        nodes = string<br/>        # Reasons can be one of Drifted, Underutilized, or Empty<br/>        # If omitted, it’s assumed that the budget applies to all reasons.<br/>        # See https://karpenter.sh/v1.1/concepts/disruption/#reasons<br/>        reasons = optional(list(string))<br/>      })), [])<br/>    }), {})<br/>    # Karpenter provisioner total CPU limit for all pods running on the EC2 instances launched by Karpenter<br/>    total_cpu_limit = string<br/>    # Karpenter provisioner total memory limit for all pods running on the EC2 instances launched by Karpenter<br/>    total_memory_limit = string<br/>    # Additional resource limits (e.g., GPU, custom resources) to merge into spec.limits. Example: {"nvidia.com/gpu" = "1"}<br/>    gpu_total_limits = optional(map(string), {})<br/>    # Set a weight for this node pool.<br/>    # See https://karpenter.sh/docs/concepts/scheduling/#weighted-nodepools<br/>    weight      = optional(number, 50)<br/>    labels      = optional(map(string))<br/>    annotations = optional(map(string))<br/>    # Karpenter provisioner taints configuration. See https://aws.github.io/aws-eks-best-practices/karpenter/#create-provisioners-that-are-mutually-exclusive for more details<br/>    taints = optional(list(object({<br/>      key    = string<br/>      effect = string<br/>      value  = optional(string)<br/>    })))<br/>    startup_taints = optional(list(object({<br/>      key    = string<br/>      effect = string<br/>      value  = optional(string)<br/>    })))<br/>    # Karpenter node metadata options. See https://karpenter.sh/docs/concepts/nodeclasses/#specmetadataoptions for more details<br/>    metadata_options = optional(object({<br/>      httpEndpoint            = optional(string, "enabled")<br/>      httpProtocolIPv6        = optional(string, "disabled")<br/>      httpPutResponseHopLimit = optional(number, 2)<br/>      # httpTokens can be either "required" or "optional"<br/>      httpTokens = optional(string, "required")<br/>    }), {})<br/>    # Enable detailed monitoring for EC2 instances. See https://karpenter.sh/docs/concepts/nodeclasses/#specdetailedmonitoring<br/>    detailed_monitoring = optional(bool, false)<br/>    # User data script to pass to EC2 instances. See https://karpenter.sh/docs/concepts/nodeclasses/#specuserdata<br/>    user_data = optional(string, null)<br/>    # ami_family dictates the default bootstrapping logic.<br/>    # It is only required if you do not specify amiSelectorTerms.alias<br/>    ami_family = optional(string, null)<br/>    # Selectors for the AMI used by Karpenter provisioner when provisioning nodes.<br/>    # Usually use { alias = "<family>@latest" } but version can be pinned instead of "latest".<br/>    # Based on the ami_selector_terms, Karpenter will automatically query for the appropriate EKS optimized AMI via AWS Systems Manager (SSM)<br/>    ami_selector_terms = list(any)<br/>    # Karpenter nodes block device mappings. Controls the Elastic Block Storage volumes that Karpenter attaches to provisioned nodes.<br/>    # Karpenter uses default block device mappings for the AMI Family specified.<br/>    # For example, the Bottlerocket AMI Family defaults with two block device mappings,<br/>    # and normally you only want to scale `/dev/xvdb` where Containers and there storage are stored.<br/>    # Most other AMIs only have one device mapping at `/dev/xvda`.<br/>    # See https://karpenter.sh/docs/concepts/nodeclasses/#specblockdevicemappings for more details<br/>    block_device_mappings = list(object({<br/>      deviceName = string<br/>      ebs = optional(object({<br/>        volumeSize          = string<br/>        volumeType          = string<br/>        deleteOnTermination = optional(bool, true)<br/>        encrypted           = optional(bool, true)<br/>        iops                = optional(number)<br/>        kmsKeyID            = optional(string, "alias/aws/ebs")<br/>        snapshotID          = optional(string)<br/>        throughput          = optional(number)<br/>      }))<br/>    }))<br/>    # Set acceptable (In) and unacceptable (Out) Kubernetes and Karpenter values for node provisioning based on Well-Known Labels and cloud-specific settings. These can include instance types, zones, computer architecture, and capacity type (such as AWS spot or on-demand). See https://karpenter.sh/v0.18.0/provisioner/#specrequirements for more details<br/>    requirements = list(object({<br/>      key      = string<br/>      operator = string<br/>      # Operators like "Exists" and "DoesNotExist" do not require a value<br/>      values = optional(list(string))<br/>    }))<br/>    # Any values for spec.template.spec.kubelet allowed by Karpenter.<br/>    # Not fully specified, because they are subject to change.<br/>    # See:<br/>    #   https://karpenter.sh/docs/concepts/nodepools/#spectemplatespeckubelet<br/>    #   https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/<br/>    kubelet = optional(any, {})<br/>  }))</pre> | n/a | yes |
| <a name="input_regex_replace_chars"></a> [regex\_replace\_chars](#input\_regex\_replace\_chars) | Terraform regular expression (regex) string.<br/>Characters matching the regex will be removed from the ID elements.<br/>If not set, `"/[^a-zA-Z0-9-]/"` is used to remove all characters other than hyphens, letters and digits. | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | AWS Region | `string` | n/a | yes |
| <a name="input_stage"></a> [stage](#input\_stage) | ID element. Usually used to indicate role, e.g. 'prod', 'staging', 'source', 'build', 'test', 'deploy', 'release' | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Additional tags (e.g. `{'BusinessUnit': 'XYZ'}`).<br/>Neither the tag keys nor the tag values will be modified by this module. | `map(string)` | `{}` | no |
| <a name="input_tenant"></a> [tenant](#input\_tenant) | ID element \_(Rarely used, not included by default)\_. A customer identifier, indicating who this instance of a resource is for | `string` | `null` | no |
| <a name="input_vpc"></a> [vpc](#input\_vpc) | VPC configuration to use when `account_map_enabled` is `false`.<br/>Provides subnet IDs for Karpenter to launch instances in. | <pre>object({<br/>    private_subnet_ids = optional(list(string), [])<br/>    public_subnet_ids  = optional(list(string), [])<br/>  })</pre> | <pre>{<br/>  "private_subnet_ids": [],<br/>  "public_subnet_ids": []<br/>}</pre> | no |
| <a name="input_vpc_component_name"></a> [vpc\_component\_name](#input\_vpc\_component\_name) | The name of the VPC component. Used to fetch VPC information from remote state<br/>when `account_map_enabled` is `true`.<br/><br/>DEPRECATED: This variable (along with account\_map\_enabled=true) is deprecated and<br/>will be removed in a future version. Set `account_map_enabled = false` and use<br/>the direct subnet ID input variables instead. | `string` | `"vpc"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_ec2_node_classes"></a> [ec2\_node\_classes](#output\_ec2\_node\_classes) | Deployed Karpenter EC2NodeClass |
| <a name="output_node_pools"></a> [node\_pools](#output\_node\_pools) | Deployed Karpenter NodePool |
<!-- markdownlint-restore -->



## References


- [https://karpenter.sh](https://karpenter.sh) -

- [https://aws.github.io/aws-eks-best-practices/karpenter](https://aws.github.io/aws-eks-best-practices/karpenter) -

- [https://karpenter.sh/docs/concepts/nodepools](https://karpenter.sh/docs/concepts/nodepools) -

- [https://aws.amazon.com/blogs/aws/introducing-karpenter-an-open-source-high-performance-kubernetes-cluster-autoscaler](https://aws.amazon.com/blogs/aws/introducing-karpenter-an-open-source-high-performance-kubernetes-cluster-autoscaler) -

- [https://github.com/aws/karpenter](https://github.com/aws/karpenter) -

- [https://ec2spotworkshops.com/karpenter.html](https://ec2spotworkshops.com/karpenter.html) -

- [https://www.eksworkshop.com/docs/autoscaling/compute/karpenter/](https://www.eksworkshop.com/docs/autoscaling/compute/karpenter/) -




[<img src="https://cloudposse.com/logo-300x69.svg" height="32" align="right"/>](https://cpco.io/homepage?utm_source=github&utm_medium=readme&utm_campaign=cloudposse-terraform-components/aws-eks-karpenter-node-pool&utm_content=)
