---
tags:
  - component/s3-bucket
  - layer/addons
  - layer/gitops
  - provider/aws
---

# Component: `s3-bucket`

This component is responsible for provisioning S3 buckets.
## Usage

**Stack Level**: Regional

Here are some example snippets for how to use this component:

`stacks/s3/defaults.yaml` file (base component for all S3 buckets with default settings):

```yaml
components:
  terraform:
    s3-bucket-defaults:
      metadata:
        type: abstract
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        enabled: true
        account_map_tenant_name: core
        # Suggested configuration for all buckets
        user_enabled: false
        acl: "private"
        grants: null
        force_destroy: false
        versioning_enabled: false
        allow_encrypted_uploads_only: true
        block_public_acls: true
        block_public_policy: true
        ignore_public_acls: true
        restrict_public_buckets: true
        allow_ssl_requests_only: true
        lifecycle_configuration_rules:
          - id: default
            enabled: true
            abort_incomplete_multipart_upload_days: 90
            filter_and:
              prefix: ""
              tags: {}
            transition:
              - storage_class: GLACIER
                days: 60
            noncurrent_version_transition:
              - storage_class: GLACIER
                days: 60
            noncurrent_version_expiration:
              days: 90
            expiration:
              days: 120
```

```yaml
import:
  - catalog/s3/defaults

components:
  terraform:
    template-bucket:
      metadata:
        component: s3-bucket
        inherits:
          - s3-bucket-defaults
      vars:
        enabled: true
        name: template
        logging_bucket_name_rendering_enabled: true
        logging:
          bucket_name: s3-access-logs
          prefix: logs/
```

<!-- prettier-ignore-start -->
<!-- prettier-ignore-end -->


<!-- markdownlint-disable -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0.0 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 4.0, < 6.0.0 |
| <a name="requirement_template"></a> [template](#requirement\_template) | >= 2.2.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 4.0, < 6.0.0 |
| <a name="provider_template"></a> [template](#provider\_template) | >= 2.2.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_account_map"></a> [account\_map](#module\_account\_map) | cloudposse/stack-config/yaml//modules/remote-state | 1.8.0 |
| <a name="module_bucket_policy"></a> [bucket\_policy](#module\_bucket\_policy) | cloudposse/iam-policy/aws | 2.0.2 |
| <a name="module_iam_roles"></a> [iam\_roles](#module\_iam\_roles) | ../account-map/modules/iam-roles | n/a |
| <a name="module_s3_bucket"></a> [s3\_bucket](#module\_s3\_bucket) | cloudposse/s3-bucket/aws | 4.10.0 |
| <a name="module_this"></a> [this](#module\_this) | cloudposse/label/null | 0.25.0 |

## Resources

| Name | Type |
|------|------|
| [aws_iam_policy_document.custom_policy](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_partition.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/partition) | data source |
| [template_file.bucket_policy](https://registry.terraform.io/providers/cloudposse/template/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_account_map"></a> [account\_map](#input\_account\_map) | Static account map to use when `account_map_enabled` is `false`. Map of account names (tenant-stage format) to account IDs.<br/>Optional attributes support component-specific functionality (e.g., audit\_account\_account\_name for cloudtrail). | <pre>object({<br/>    full_account_map              = map(string)<br/>    audit_account_account_name    = optional(string, "")<br/>    root_account_account_name     = optional(string, "")<br/>    identity_account_account_name = optional(string, "")<br/>    aws_partition                 = optional(string, "aws")<br/>    iam_role_arn_templates        = optional(map(string), {})<br/>  })</pre> | <pre>{<br/>  "audit_account_account_name": "",<br/>  "aws_partition": "aws",<br/>  "full_account_map": {},<br/>  "iam_role_arn_templates": {},<br/>  "identity_account_account_name": "",<br/>  "root_account_account_name": ""<br/>}</pre> | no |
| <a name="input_account_map_component_name"></a> [account\_map\_component\_name](#input\_account\_map\_component\_name) | The name of the account-map component | `string` | `"account-map"` | no |
| <a name="input_account_map_enabled"></a> [account\_map\_enabled](#input\_account\_map\_enabled) | Enable the account map component lookup. When disabled, use the `account_map` variable to provide static account mapping. | `bool` | `true` | no |
| <a name="input_account_map_environment_name"></a> [account\_map\_environment\_name](#input\_account\_map\_environment\_name) | The name of the environment where `account_map` is provisioned | `string` | `"gbl"` | no |
| <a name="input_account_map_stage_name"></a> [account\_map\_stage\_name](#input\_account\_map\_stage\_name) | The name of the stage where `account_map` is provisioned | `string` | `"root"` | no |
| <a name="input_account_map_tenant_name"></a> [account\_map\_tenant\_name](#input\_account\_map\_tenant\_name) | The name of the tenant where `account_map` is provisioned.<br/><br/>If the `tenant` label is not used, leave this as `null`. | `string` | `null` | no |
| <a name="input_acl"></a> [acl](#input\_acl) | The [canned ACL](https://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl) to apply.<br/>We recommend `private` to avoid exposing sensitive information. Conflicts with `grants`. | `string` | `"private"` | no |
| <a name="input_additional_tag_map"></a> [additional\_tag\_map](#input\_additional\_tag\_map) | Additional key-value pairs to add to each map in `tags_as_list_of_maps`. Not added to `tags` or `id`.<br/>This is for some rare cases where resources want additional configuration of tags<br/>and therefore take a list of maps with tag key, value, and additional configuration. | `map(string)` | `{}` | no |
| <a name="input_allow_encrypted_uploads_only"></a> [allow\_encrypted\_uploads\_only](#input\_allow\_encrypted\_uploads\_only) | Set to `true` to prevent uploads of unencrypted objects to S3 bucket | `bool` | `false` | no |
| <a name="input_allow_ssl_requests_only"></a> [allow\_ssl\_requests\_only](#input\_allow\_ssl\_requests\_only) | Set to `true` to require requests to use Secure Socket Layer (HTTPS/SSL). This will explicitly deny access to HTTP requests | `bool` | `false` | no |
| <a name="input_allowed_bucket_actions"></a> [allowed\_bucket\_actions](#input\_allowed\_bucket\_actions) | List of actions the user is permitted to perform on the S3 bucket | `list(string)` | <pre>[<br/>  "s3:PutObject",<br/>  "s3:PutObjectAcl",<br/>  "s3:GetObject",<br/>  "s3:DeleteObject",<br/>  "s3:ListBucket",<br/>  "s3:ListBucketMultipartUploads",<br/>  "s3:GetBucketLocation",<br/>  "s3:AbortMultipartUpload"<br/>]</pre> | no |
| <a name="input_attributes"></a> [attributes](#input\_attributes) | ID element. Additional attributes (e.g. `workers` or `cluster`) to add to `id`,<br/>in the order they appear in the list. New attributes are appended to the<br/>end of the list. The elements of the list are joined by the `delimiter`<br/>and treated as a single ID element. | `list(string)` | `[]` | no |
| <a name="input_block_public_acls"></a> [block\_public\_acls](#input\_block\_public\_acls) | Set to `false` to disable the blocking of new public access lists on the bucket | `bool` | `true` | no |
| <a name="input_block_public_policy"></a> [block\_public\_policy](#input\_block\_public\_policy) | Set to `false` to disable the blocking of new public policies on the bucket | `bool` | `true` | no |
| <a name="input_bucket_key_enabled"></a> [bucket\_key\_enabled](#input\_bucket\_key\_enabled) | Set this to true to use Amazon S3 Bucket Keys for SSE-KMS, which reduce the cost of AWS KMS requests.<br/>For more information, see: https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucket-key.html | `bool` | `false` | no |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Bucket name. If provided, the bucket will be created with this name instead of generating the name from the context | `string` | `""` | no |
| <a name="input_context"></a> [context](#input\_context) | Single object for setting entire context at once.<br/>See description of individual variables for details.<br/>Leave string and numeric variables as `null` to use default value.<br/>Individual variable settings (non-null) override settings in context object,<br/>except for attributes, tags, and additional\_tag\_map, which are merged. | `any` | <pre>{<br/>  "additional_tag_map": {},<br/>  "attributes": [],<br/>  "delimiter": null,<br/>  "descriptor_formats": {},<br/>  "enabled": true,<br/>  "environment": null,<br/>  "id_length_limit": null,<br/>  "label_key_case": null,<br/>  "label_order": [],<br/>  "label_value_case": null,<br/>  "labels_as_tags": [<br/>    "unset"<br/>  ],<br/>  "name": null,<br/>  "namespace": null,<br/>  "regex_replace_chars": null,<br/>  "stage": null,<br/>  "tags": {},<br/>  "tenant": null<br/>}</pre> | no |
| <a name="input_cors_configuration"></a> [cors\_configuration](#input\_cors\_configuration) | Specifies the allowed headers, methods, origins and exposed headers when using CORS on this bucket | <pre>list(object({<br/>    allowed_headers = list(string)<br/>    allowed_methods = list(string)<br/>    allowed_origins = list(string)<br/>    expose_headers  = list(string)<br/>    max_age_seconds = number<br/>  }))</pre> | `null` | no |
| <a name="input_custom_policy_account_names"></a> [custom\_policy\_account\_names](#input\_custom\_policy\_account\_names) | List of accounts names to assign as principals for the s3 bucket custom policy | `list(string)` | `[]` | no |
| <a name="input_custom_policy_actions"></a> [custom\_policy\_actions](#input\_custom\_policy\_actions) | List of S3 Actions for the custom policy | `list(string)` | `[]` | no |
| <a name="input_custom_policy_enabled"></a> [custom\_policy\_enabled](#input\_custom\_policy\_enabled) | Whether to enable or disable the custom policy. If enabled, the default policy will be ignored | `bool` | `false` | no |
| <a name="input_delimiter"></a> [delimiter](#input\_delimiter) | Delimiter to be used between ID elements.<br/>Defaults to `-` (hyphen). Set to `""` to use no delimiter at all. | `string` | `null` | no |
| <a name="input_descriptor_formats"></a> [descriptor\_formats](#input\_descriptor\_formats) | Describe additional descriptors to be output in the `descriptors` output map.<br/>Map of maps. Keys are names of descriptors. Values are maps of the form<br/>`{<br/>  format = string<br/>  labels = list(string)<br/>}`<br/>(Type is `any` so the map values can later be enhanced to provide additional options.)<br/>`format` is a Terraform format string to be passed to the `format()` function.<br/>`labels` is a list of labels, in order, to pass to `format()` function.<br/>Label values will be normalized before being passed to `format()` so they will be<br/>identical to how they appear in `id`.<br/>Default is `{}` (`descriptors` output will be empty). | `any` | `{}` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `null` | no |
| <a name="input_environment"></a> [environment](#input\_environment) | ID element. Usually used for region e.g. 'uw2', 'us-west-2', OR role 'prod', 'staging', 'dev', 'UAT' | `string` | `null` | no |
| <a name="input_event_notification_details"></a> [event\_notification\_details](#input\_event\_notification\_details) | S3 event notification details | <pre>object({<br/>    enabled     = bool<br/>    eventbridge = optional(bool, false)<br/>    lambda_list = optional(list(object({<br/>      lambda_function_arn = string<br/>      events              = optional(list(string), ["s3:ObjectCreated:*"])<br/>      filter_prefix       = optional(string)<br/>      filter_suffix       = optional(string)<br/>    })), [])<br/><br/>    queue_list = optional(list(object({<br/>      queue_arn     = string<br/>      events        = optional(list(string), ["s3:ObjectCreated:*"])<br/>      filter_prefix = optional(string)<br/>      filter_suffix = optional(string)<br/>    })), [])<br/><br/>    topic_list = optional(list(object({<br/>      topic_arn     = string<br/>      events        = optional(list(string), ["s3:ObjectCreated:*"])<br/>      filter_prefix = optional(string)<br/>      filter_suffix = optional(string)<br/>    })), [])<br/>  })</pre> | <pre>{<br/>  "enabled": false<br/>}</pre> | no |
| <a name="input_force_destroy"></a> [force\_destroy](#input\_force\_destroy) | When `true`, permits a non-empty S3 bucket to be deleted by first deleting all objects in the bucket.<br/>THESE OBJECTS ARE NOT RECOVERABLE even if they were versioned and stored in Glacier. | `bool` | `false` | no |
| <a name="input_grants"></a> [grants](#input\_grants) | A list of policy grants for the bucket, taking a list of permissions.<br/>Conflicts with `acl`. Set `acl` to `null` to use this. | <pre>list(object({<br/>    id          = string<br/>    type        = string<br/>    permissions = list(string)<br/>    uri         = string<br/>  }))</pre> | `[]` | no |
| <a name="input_iam_policy_statements"></a> [iam\_policy\_statements](#input\_iam\_policy\_statements) | Map of IAM policy statements to use in the bucket policy. | `any` | `{}` | no |
| <a name="input_id_length_limit"></a> [id\_length\_limit](#input\_id\_length\_limit) | Limit `id` to this many characters (minimum 6).<br/>Set to `0` for unlimited length.<br/>Set to `null` for keep the existing setting, which defaults to `0`.<br/>Does not affect `id_full`. | `number` | `null` | no |
| <a name="input_ignore_public_acls"></a> [ignore\_public\_acls](#input\_ignore\_public\_acls) | Set to `false` to disable the ignoring of public access lists on the bucket | `bool` | `true` | no |
| <a name="input_kms_master_key_arn"></a> [kms\_master\_key\_arn](#input\_kms\_master\_key\_arn) | The AWS KMS master key ARN used for the `SSE-KMS` encryption. This can only be used when you set the value of `sse_algorithm` as `aws:kms`. The default aws/s3 AWS KMS master key is used if this element is absent while the `sse_algorithm` is `aws:kms` | `string` | `""` | no |
| <a name="input_label_key_case"></a> [label\_key\_case](#input\_label\_key\_case) | Controls the letter case of the `tags` keys (label names) for tags generated by this module.<br/>Does not affect keys of tags passed in via the `tags` input.<br/>Possible values: `lower`, `title`, `upper`.<br/>Default value: `title`. | `string` | `null` | no |
| <a name="input_label_order"></a> [label\_order](#input\_label\_order) | The order in which the labels (ID elements) appear in the `id`.<br/>Defaults to ["namespace", "environment", "stage", "name", "attributes"].<br/>You can omit any of the 6 labels ("tenant" is the 6th), but at least one must be present. | `list(string)` | `null` | no |
| <a name="input_label_value_case"></a> [label\_value\_case](#input\_label\_value\_case) | Controls the letter case of ID elements (labels) as included in `id`,<br/>set as tag values, and output by this module individually.<br/>Does not affect values of tags passed in via the `tags` input.<br/>Possible values: `lower`, `title`, `upper` and `none` (no transformation).<br/>Set this to `title` and set `delimiter` to `""` to yield Pascal Case IDs.<br/>Default value: `lower`. | `string` | `null` | no |
| <a name="input_labels_as_tags"></a> [labels\_as\_tags](#input\_labels\_as\_tags) | Set of labels (ID elements) to include as tags in the `tags` output.<br/>Default is to include all labels.<br/>Tags with empty values will not be included in the `tags` output.<br/>Set to `[]` to suppress all generated tags.<br/>**Notes:**<br/>  The value of the `name` tag, if included, will be the `id`, not the `name`.<br/>  Unlike other `null-label` inputs, the initial setting of `labels_as_tags` cannot be<br/>  changed in later chained modules. Attempts to change it will be silently ignored. | `set(string)` | <pre>[<br/>  "default"<br/>]</pre> | no |
| <a name="input_lifecycle_configuration_rules"></a> [lifecycle\_configuration\_rules](#input\_lifecycle\_configuration\_rules) | A list of lifecycle V2 rules | <pre>list(object({<br/>    enabled = bool<br/>    id      = string<br/><br/>    abort_incomplete_multipart_upload_days = number<br/><br/>    # `filter_and` is the `and` configuration block inside the `filter` configuration.<br/>    # This is the only place you should specify a prefix.<br/>    filter_and = any<br/>    expiration = any<br/>    transition = list(any)<br/><br/>    noncurrent_version_expiration = any<br/>    noncurrent_version_transition = list(any)<br/>  }))</pre> | `[]` | no |
| <a name="input_logging"></a> [logging](#input\_logging) | Bucket access logging configuration. | <pre>object({<br/>    bucket_name = string<br/>    prefix      = string<br/>  })</pre> | `null` | no |
| <a name="input_logging_bucket_name_rendering_enabled"></a> [logging\_bucket\_name\_rendering\_enabled](#input\_logging\_bucket\_name\_rendering\_enabled) | Whether to render the logging bucket name, prepending context | `bool` | `false` | no |
| <a name="input_logging_bucket_name_rendering_template"></a> [logging\_bucket\_name\_rendering\_template](#input\_logging\_bucket\_name\_rendering\_template) | The template for the template used to render Bucket Name for the Logging bucket.<br/>Default is appropriate when using `tenant` and default label order with `null-label`.<br/>Use `"%s-%s-%s-%%s"` when not using `tenant`. | `string` | `"%s-%s-%s-%s-%s"` | no |
| <a name="input_logging_bucket_prefix_rendering_template"></a> [logging\_bucket\_prefix\_rendering\_template](#input\_logging\_bucket\_prefix\_rendering\_template) | The template for the template used to render Bucket Prefix for the Logging bucket, uses the format `var.logging.prefix`/`var.name` | `string` | `"%s/%s/"` | no |
| <a name="input_name"></a> [name](#input\_name) | ID element. Usually the component or solution name, e.g. 'app' or 'jenkins'.<br/>This is the only ID element not also included as a `tag`.<br/>The "name" tag is set to the full `id` string. There is no tag with the value of the `name` input. | `string` | `null` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | ID element. Usually an abbreviation of your organization name, e.g. 'eg' or 'cp', to help ensure generated IDs are globally unique | `string` | `null` | no |
| <a name="input_object_lock_configuration"></a> [object\_lock\_configuration](#input\_object\_lock\_configuration) | A configuration for S3 object locking. With S3 Object Lock, you can store objects using a `write once, read many` (WORM) model. Object Lock can help prevent objects from being deleted or overwritten for a fixed amount of time or indefinitely. | <pre>object({<br/>    mode  = string # Valid values are GOVERNANCE and COMPLIANCE.<br/>    days  = number<br/>    years = number<br/>  })</pre> | `null` | no |
| <a name="input_privileged_principal_actions"></a> [privileged\_principal\_actions](#input\_privileged\_principal\_actions) | List of actions to permit `privileged_principal_arns` to perform on bucket and bucket prefixes (see `privileged_principal_arns`) | `list(string)` | `[]` | no |
| <a name="input_privileged_principal_arns"></a> [privileged\_principal\_arns](#input\_privileged\_principal\_arns) | List of maps. Each map has one key, an IAM Principal ARN, whose associated value is<br/>a list of S3 path prefixes to grant `privileged_principal_actions` permissions for that principal,<br/>in addition to the bucket itself, which is automatically included. Prefixes should not begin with '/'. | `list(map(list(string)))` | `[]` | no |
| <a name="input_regex_replace_chars"></a> [regex\_replace\_chars](#input\_regex\_replace\_chars) | Terraform regular expression (regex) string.<br/>Characters matching the regex will be removed from the ID elements.<br/>If not set, `"/[^a-zA-Z0-9-]/"` is used to remove all characters other than hyphens, letters and digits. | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | AWS Region | `string` | n/a | yes |
| <a name="input_restrict_public_buckets"></a> [restrict\_public\_buckets](#input\_restrict\_public\_buckets) | Set to `false` to disable the restricting of making the bucket public | `bool` | `true` | no |
| <a name="input_s3_object_ownership"></a> [s3\_object\_ownership](#input\_s3\_object\_ownership) | Specifies the S3 object ownership control. Valid values are `ObjectWriter`, `BucketOwnerPreferred`, and 'BucketOwnerEnforced'. | `string` | `"ObjectWriter"` | no |
| <a name="input_s3_replica_bucket_arn"></a> [s3\_replica\_bucket\_arn](#input\_s3\_replica\_bucket\_arn) | A single S3 bucket ARN to use for all replication rules.<br/>Note: The destination bucket can be specified in the replication rule itself<br/>(which allows for multiple destinations), in which case it will take precedence over this variable. | `string` | `""` | no |
| <a name="input_s3_replication_enabled"></a> [s3\_replication\_enabled](#input\_s3\_replication\_enabled) | Set this to true and specify `s3_replication_rules` to enable replication. `versioning_enabled` must also be `true`. | `bool` | `false` | no |
| <a name="input_s3_replication_rules"></a> [s3\_replication\_rules](#input\_s3\_replication\_rules) | Specifies the replication rules for S3 bucket replication if enabled. You must also set s3\_replication\_enabled to true. | `list(any)` | `null` | no |
| <a name="input_s3_replication_source_roles"></a> [s3\_replication\_source\_roles](#input\_s3\_replication\_source\_roles) | Cross-account IAM Role ARNs that will be allowed to perform S3 replication to this bucket (for replication within the same AWS account, it's not necessary to adjust the bucket policy). | `list(string)` | `[]` | no |
| <a name="input_source_policy_documents"></a> [source\_policy\_documents](#input\_source\_policy\_documents) | List of IAM policy documents that are merged together into the exported document.<br/>Statements defined in source\_policy\_documents or source\_json must have unique SIDs.<br/>Statement having SIDs that match policy SIDs generated by this module will override them. | `list(string)` | `[]` | no |
| <a name="input_sse_algorithm"></a> [sse\_algorithm](#input\_sse\_algorithm) | The server-side encryption algorithm to use. Valid values are `AES256` and `aws:kms` | `string` | `"AES256"` | no |
| <a name="input_stage"></a> [stage](#input\_stage) | ID element. Usually used to indicate role, e.g. 'prod', 'staging', 'source', 'build', 'test', 'deploy', 'release' | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Additional tags (e.g. `{'BusinessUnit': 'XYZ'}`).<br/>Neither the tag keys nor the tag values will be modified by this module. | `map(string)` | `{}` | no |
| <a name="input_tenant"></a> [tenant](#input\_tenant) | ID element \_(Rarely used, not included by default)\_. A customer identifier, indicating who this instance of a resource is for | `string` | `null` | no |
| <a name="input_transfer_acceleration_enabled"></a> [transfer\_acceleration\_enabled](#input\_transfer\_acceleration\_enabled) | Set this to true to enable S3 Transfer Acceleration for the bucket. | `bool` | `false` | no |
| <a name="input_user_enabled"></a> [user\_enabled](#input\_user\_enabled) | Set to `true` to create an IAM user with permission to access the bucket | `bool` | `false` | no |
| <a name="input_versioning_enabled"></a> [versioning\_enabled](#input\_versioning\_enabled) | A state of versioning. Versioning is a means of keeping multiple variants of an object in the same bucket | `bool` | `true` | no |
| <a name="input_website_inputs"></a> [website\_inputs](#input\_website\_inputs) | Specifies the static website hosting configuration object. | <pre>list(object({<br/>    index_document           = string<br/>    error_document           = string<br/>    redirect_all_requests_to = string<br/>    routing_rules            = string<br/>  }))</pre> | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bucket_arn"></a> [bucket\_arn](#output\_bucket\_arn) | Bucket ARN |
| <a name="output_bucket_domain_name"></a> [bucket\_domain\_name](#output\_bucket\_domain\_name) | Bucket domain name |
| <a name="output_bucket_id"></a> [bucket\_id](#output\_bucket\_id) | Bucket ID |
| <a name="output_bucket_region"></a> [bucket\_region](#output\_bucket\_region) | Bucket region |
| <a name="output_bucket_regional_domain_name"></a> [bucket\_regional\_domain\_name](#output\_bucket\_regional\_domain\_name) | Bucket region-specific domain name |
<!-- markdownlint-restore -->



## References


- [cloudposse-terraform-components](https://github.com/orgs/cloudposse-terraform-components/repositories) - Cloud Posse's upstream component




[<img src="https://cloudposse.com/logo-300x69.svg" height="32" align="right"/>](https://cpco.io/homepage?utm_source=github&utm_medium=readme&utm_campaign=cloudposse-terraform-components/aws-s3-bucket&utm_content=)
