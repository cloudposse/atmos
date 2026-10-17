# Native Terraform README

Custom formatting from the recovered Cloud Posse README design.



## Version Requirements

| Requirement | Version |
| --- | --- |
| `terraform` | ![>= 1.5.0](https://img.shields.io/badge/>=_1.5.0-success.svg?style=for-the-badge) |
| `aws` | ![>= 5.0](https://img.shields.io/badge/>=_5.0-success.svg?style=for-the-badge) |


## Providers

| Provider | Version |
| --- | --- |
| [`aws`](https://registry.terraform.io/providers/aws/latest) | ![>= 5.0](https://img.shields.io/badge/>=_5.0-success.svg?style=for-the-badge) |


## Modules

Name | Version | Source | Description
--- | --- | --- | ---
`helper` | [![latest](https://img.shields.io/badge/latest-success.svg?style=for-the-badge)](../helper) | [`../helper`](../helper) | Local helper module.
`label` | [![0.25.0](https://img.shields.io/badge/0.25.0-success.svg?style=for-the-badge)](https://registry.terraform.io/modules/cloudposse/label/null/0.25.0) | [`cloudposse/label/null`](https://registry.terraform.io/modules/cloudposse/label/null/0.25.0) | Shared naming convention.
`state` | [![1.5.0](https://img.shields.io/badge/1.5.0-success.svg?style=for-the-badge)](https://registry.terraform.io/modules/cloudposse/stack-config/yaml/1.5.0/submodules/remote-state) | [`cloudposse/stack-config/yaml//modules/remote-state`](https://registry.terraform.io/modules/cloudposse/stack-config/yaml/1.5.0/submodules/remote-state) | Remote state submodule.


## Resources

The following resources are used by this module:

  - [`aws_s3_bucket.assets`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket) (resource)(./main.tf#12)

## Data Sources

The following data sources are used by this module:

  - [`aws_caller_identity.current`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) (data source)

## Outputs

<dl>
  <dt><code>bucket_id</code></dt>
  <dd>


  Identifier of the assets bucket.<br/>
  Use this when configuring access policies.<br/>
  <br/>

  </dd>
  <dt><code>legacy_id</code> <img src="https://img.shields.io/badge/OBSOLETE-f44336.svg?style=for-the-badge"/></dt>
  <dd>


  Use bucket_id instead.<br/>

  </dd>
</dl>

## Required Variables

Required variables are the minimum set of variables that must be set to use this module.

> [!IMPORTANT]
>
> To customize the names and tags of the resources created by this module, see the [context variables](#context-variables).
>
### `bucket_name` (`string`) <i>required</i>


Name of the assets bucket.<br/>
Must be unique across AWS.<br/>
<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>Yes</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>string</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>unset</code>
>   </dd>
> </dl>
>



## Optional Variables
### `configuration` <i>optional</i>


Settings for the application.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>No</dd>
>   <dt>Type</dt>
>   <dd>
>
>
>   ```hcl
>   object({
>       enabled = bool
>       ports   = list(number)
>     })
>   ```
>
>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>
>
>   ```hcl
>   {
>     "enabled": true,
>     "ports": [
>       80,
>       443
>     ]
>   }
>   ```
>
>   </dd>
> </dl>
>


### `legacy_mode` (`bool`) <i>optional</i> <img src="https://img.shields.io/badge/OBSOLETE-f44336.svg?style=for-the-badge"/>


Use configuration instead.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>No</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>bool</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>false</code>
>   </dd>
> </dl>
>


### `optional_note` (`string`) <i>optional</i>


An optional note. Null disables the note.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>No</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>string</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>null</code>
>   </dd>
> </dl>
>


### `replicas` (`number`) <i>optional</i>


Number of replicas.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>No</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>number</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>2</code>
>   </dd>
> </dl>
>



## Context Variables

The following variables are defined in the `context.tf` file of this module and part of the [terraform-null-label](https://registry.terraform.io/modules/cloudposse/label/null) pattern. These are identical in all Cloud Posse modules.

<details>
<summary>Click to expand</summary>


### `namespace` (`string`) <i>required</i>


Namespace used in resource names.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>Yes</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>string</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>unset</code>
>   </dd>
> </dl>
>


### `tags` (`map(string)`) <i>optional</i>


Additional resource tags.<br/>

>
> <dl>
>   <dt>Required</dt>
>   <dd>No</dd>
>   <dt>Type</dt>
>   <dd>
>   <code>map(string)</code>
>   </dd>
>
>   <dt>Default value</dt>
>   <dd>
>   <code>{}</code>
>   </dd>
> </dl>
>



</details>
