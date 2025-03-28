My Example Atmos Terraform Module

Test Terraform.


Extra info: Testing Terraform Docs integration



## Terraform Docs
## Requirements

The following requirements are needed by this module:

- <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) (>= 1.0.0)

- <a name="requirement_aws"></a> [aws](#requirement\_aws) (>= 4.9.0)

## Providers

The following providers are used by this module:

- <a name="provider_aws"></a> [aws](#provider\_aws) (>= 4.9.0)

## Modules

The following Modules are called:

### <a name="module_endpoint_security_groups"></a> [endpoint\_security\_groups](#module\_endpoint\_security\_groups)

Source: cloudposse/security-group/aws

Version: 2.2.0

### <a name="module_subnets"></a> [subnets](#module\_subnets)

Source: cloudposse/dynamic-subnets/aws

Version: 2.3.0

### <a name="module_this"></a> [this](#module\_this)

Source: cloudposse/label/null

Version: 0.25.0

### <a name="module_utils"></a> [utils](#module\_utils)

Source: cloudposse/utils/aws

Version: 1.4.0

### <a name="module_vpc"></a> [vpc](#module\_vpc)

Source: cloudposse/vpc/aws

Version: 2.1.1

### <a name="module_vpc_endpoints"></a> [vpc\_endpoints](#module\_vpc\_endpoints)

Source: cloudposse/vpc/aws//modules/vpc-endpoints

Version: 2.1.0

### <a name="module_vpc_flow_logs_bucket"></a> [vpc\_flow\_logs\_bucket](#module\_vpc\_flow\_logs\_bucket)

Source: cloudposse/stack-config/yaml//modules/remote-state

Version: 1.5.0

## Resources

The following resources are used by this module:

- [aws_flow_log.default](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/flow_log) (resource)
- [aws_shield_protection.nat_eip_shield_protection](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/shield_protection) (resource)
- [aws_caller_identity.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) (data source)
- [aws_eip.eip](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/eip) (data source)

## Required Inputs

The following input variables are required:

### <a name="input_region"></a> [region](#input\_region)

Description: AWS Region

Type: `string`

### <a name="input_subnet_type_tag_key"></a> [subnet\_type\_tag\_key](#input\_subnet\_type\_tag\_key)

Description: Key for subnet type tag to provide information about the type of subnets, e.g. `cpco/subnet/type=private` or `cpcp/subnet/type=public`

Type: `string`

## Optional Inputs

The following input variables are optional (have default values):

### <a name="input_additional_tag_map"></a> [additional\_tag\_map](#input\_additional\_tag\_map)

Description: Additional key-value pairs to add to each map in `tags_as_list_of_maps`. Not added to `tags` or `id`.  
This is for some rare cases where resources want additional configuration of tags  
and therefore take a list of maps with tag key, value, and additional configuration.

Type: `map(string)`

Default: `{}`

### <a name="input_assign_generated_ipv6_cidr_block"></a> [assign\_generated\_ipv6\_cidr\_block](#input\_assign\_generated\_ipv6\_cidr\_block)

Description: When `true`, assign AWS generated IPv6 CIDR block to the VPC.  Conflicts with `ipv6_ipam_pool_id`.

Type: `bool`

Default: `false`

### <a name="input_attributes"></a> [attributes](#input\_attributes)

Description: ID element. Additional attributes (e.g. `workers` or `cluster`) to add to `id`,  
in the order they appear in the list. New attributes are appended to the  
end of the list. The elements of the list are joined by the `delimiter`  
and treated as a single ID element.

Type: `list(string)`

Default: `[]`

### <a name="input_availability_zone_ids"></a> [availability\_zone\_ids](#input\_availability\_zone\_ids)

Description: List of Availability Zones IDs where subnets will be created. Overrides `availability_zones`.  
Can be the full name, e.g. `use1-az1`, or just the part after the AZ ID region code, e.g. `-az1`,  
to allow reusable values across regions. Consider contention for resources and spot pricing in each AZ when selecting.  
Useful in some regions when using only some AZs and you want to use the same ones across multiple accounts.

Type: `list(string)`

Default: `[]`

### <a name="input_availability_zones"></a> [availability\_zones](#input\_availability\_zones)

Description: List of Availability Zones (AZs) where subnets will be created. Ignored when `availability_zone_ids` is set.  
Can be the full name, e.g. `us-east-1a`, or just the part after the region, e.g. `a` to allow reusable values across regions.  
The order of zones in the list ***must be stable*** or else Terraform will continually make changes.  
If no AZs are specified, then `max_subnet_count` AZs will be selected in alphabetical order.  
If `max_subnet_count > 0` and `length(var.availability_zones) > max_subnet_count`, the list  
will be truncated. We recommend setting `availability_zones` and `max_subnet_count` explicitly as constant
(not computed) values for predictability, consistency, and stability.

Type: `list(string)`

Default: `[]`

### <a name="input_context"></a> [context](#input\_context)

Description: Single object for setting entire context at once.  
See description of individual variables for details.  
Leave string and numeric variables as `null` to use default value.  
Individual variable settings (non-null) override settings in context object,  
except for attributes, tags, and additional\_tag\_map, which are merged.

Type: `any`

Default:

```json
{
  "additional_tag_map": {},
  "attributes": [],
  "delimiter": null,
  "descriptor_formats": {},
  "enabled": true,
  "environment": null,
  "id_length_limit": null,
  "label_key_case": null,
  "label_order": [],
  "label_value_case": null,
  "labels_as_tags": [
    "unset"
  ],
  "name": null,
  "namespace": null,
  "regex_replace_chars": null,
  "stage": null,
  "tags": {},
  "tenant": null
}
```

### <a name="input_delimiter"></a> [delimiter](#input\_delimiter)

Description: Delimiter to be used between ID elements.  
Defaults to `-` (hyphen). Set to `""` to use no delimiter at all.

Type: `string`

Default: `null`

### <a name="input_descriptor_formats"></a> [descriptor\_formats](#input\_descriptor\_formats)

Description: Describe additional descriptors to be output in the `descriptors` output map.  
Map of maps. Keys are names of descriptors. Values are maps of the form
`{  
   format = string  
   labels = list(string)
}`
(Type is `any` so the map values can later be enhanced to provide additional options.)
`format` is a Terraform format string to be passed to the `format()` function.
`labels` is a list of labels, in order, to pass to `format()` function.  
Label values will be normalized before being passed to `format()` so they will be  
identical to how they appear in `id`.  
Default is `{}` (`descriptors` output will be empty).

Type: `any`

Default: `{}`

### <a name="input_enabled"></a> [enabled](#input\_enabled)

Description: Set to false to prevent the module from creating any resources

Type: `bool`

Default: `null`

### <a name="input_environment"></a> [environment](#input\_environment)

Description: ID element. Usually used for region e.g. 'uw2', 'us-west-2', OR role 'prod', 'staging', 'dev', 'UAT'

Type: `string`

Default: `null`

### <a name="input_gateway_vpc_endpoints"></a> [gateway\_vpc\_endpoints](#input\_gateway\_vpc\_endpoints)

Description: A list of Gateway VPC Endpoints to provision into the VPC. Only valid values are "dynamodb" and "s3".

Type: `set(string)`

Default: `[]`

### <a name="input_id_length_limit"></a> [id\_length\_limit](#input\_id\_length\_limit)

Description: Limit `id` to this many characters (minimum 6).  
Set to `0` for unlimited length.  
Set to `null` for keep the existing setting, which defaults to `0`.  
Does not affect `id_full`.

Type: `number`

Default: `null`

### <a name="input_interface_vpc_endpoints"></a> [interface\_vpc\_endpoints](#input\_interface\_vpc\_endpoints)

Description: A list of Interface VPC Endpoints to provision into the VPC.

Type: `set(string)`

Default: `[]`

### <a name="input_ipv4_additional_cidr_block_associations"></a> [ipv4\_additional\_cidr\_block\_associations](#input\_ipv4\_additional\_cidr\_block\_associations)

Description: IPv4 CIDR blocks to assign to the VPC.
`ipv4_cidr_block` can be set explicitly, or set to `null` with the CIDR block derived from `ipv4_ipam_pool_id` using `ipv4_netmask_length`.  
Map keys must be known at `plan` time, and are only used to track changes.

Type:

```hcl
map(object({
    ipv4_cidr_block     = string
    ipv4_ipam_pool_id   = string
    ipv4_netmask_length = number
  }))
```

Default: `{}`

### <a name="input_ipv4_cidr_block_association_timeouts"></a> [ipv4\_cidr\_block\_association\_timeouts](#input\_ipv4\_cidr\_block\_association\_timeouts)

Description: Timeouts (in `go` duration format) for creating and destroying IPv4 CIDR block associations

Type:

```hcl
object({
    create = string
    delete = string
  })
```

Default: `null`

### <a name="input_ipv4_cidrs"></a> [ipv4\_cidrs](#input\_ipv4\_cidrs)

Description: Lists of CIDRs to assign to subnets. Order of CIDRs in the lists must not change over time.  
Lists may contain more CIDRs than needed.

Type:

```hcl
list(object({
    private = list(string)
    public  = list(string)
  }))
```

Default: `[]`

### <a name="input_ipv4_primary_cidr_block"></a> [ipv4\_primary\_cidr\_block](#input\_ipv4\_primary\_cidr\_block)

Description: The primary IPv4 CIDR block for the VPC.  
Either `ipv4_primary_cidr_block` or `ipv4_primary_cidr_block_association` must be set, but not both.

Type: `string`

Default: `null`

### <a name="input_ipv4_primary_cidr_block_association"></a> [ipv4\_primary\_cidr\_block\_association](#input\_ipv4\_primary\_cidr\_block\_association)

Description: Configuration of the VPC's primary IPv4 CIDR block via IPAM. Conflicts with `ipv4_primary_cidr_block`.  
One of `ipv4_primary_cidr_block` or `ipv4_primary_cidr_block_association` must be set.  
Additional CIDR blocks can be set via `ipv4_additional_cidr_block_associations`.

Type:

```hcl
object({
    ipv4_ipam_pool_id   = string
    ipv4_netmask_length = number
  })
```

Default: `null`

### <a name="input_label_key_case"></a> [label\_key\_case](#input\_label\_key\_case)

Description: Controls the letter case of the `tags` keys (label names) for tags generated by this module.  
Does not affect keys of tags passed in via the `tags` input.  
Possible values: `lower`, `title`, `upper`.  
Default value: `title`.

Type: `string`

Default: `null`

### <a name="input_label_order"></a> [label\_order](#input\_label\_order)

Description: The order in which the labels (ID elements) appear in the `id`.  
Defaults to ["namespace", "environment", "stage", "name", "attributes"].  
You can omit any of the 6 labels ("tenant" is the 6th), but at least one must be present.

Type: `list(string)`

Default: `null`

### <a name="input_label_value_case"></a> [label\_value\_case](#input\_label\_value\_case)

Description: Controls the letter case of ID elements (labels) as included in `id`,  
set as tag values, and output by this module individually.  
Does not affect values of tags passed in via the `tags` input.  
Possible values: `lower`, `title`, `upper` and `none` (no transformation).  
Set this to `title` and set `delimiter` to `""` to yield Pascal Case IDs.  
Default value: `lower`.

Type: `string`

Default: `null`

### <a name="input_labels_as_tags"></a> [labels\_as\_tags](#input\_labels\_as\_tags)

Description: Set of labels (ID elements) to include as tags in the `tags` output.  
Default is to include all labels.  
Tags with empty values will not be included in the `tags` output.  
Set to `[]` to suppress all generated tags.
**Notes:**  
  The value of the `name` tag, if included, will be the `id`, not the `name`.  
  Unlike other `null-label` inputs, the initial setting of `labels_as_tags` cannot be  
  changed in later chained modules. Attempts to change it will be silently ignored.

Type: `set(string)`

Default:

```json
[
  "default"
]
```

### <a name="input_map_public_ip_on_launch"></a> [map\_public\_ip\_on\_launch](#input\_map\_public\_ip\_on\_launch)

Description: Instances launched into a public subnet should be assigned a public IP address

Type: `bool`

Default: `true`

### <a name="input_max_subnet_count"></a> [max\_subnet\_count](#input\_max\_subnet\_count)

Description: Sets the maximum amount of subnets to deploy. 0 will deploy a subnet for every provided availability zone (in `region_availability_zones` variable) within the region

Type: `number`

Default: `0`

### <a name="input_name"></a> [name](#input\_name)

Description: ID element. Usually the component or solution name, e.g. 'app' or 'jenkins'.  
This is the only ID element not also included as a `tag`.  
The "name" tag is set to the full `id` string. There is no tag with the value of the `name` input.

Type: `string`

Default: `null`

### <a name="input_namespace"></a> [namespace](#input\_namespace)

Description: ID element. Usually an abbreviation of your organization name, e.g. 'eg' or 'cp', to help ensure generated IDs are globally unique

Type: `string`

Default: `null`

### <a name="input_nat_eip_aws_shield_protection_enabled"></a> [nat\_eip\_aws\_shield\_protection\_enabled](#input\_nat\_eip\_aws\_shield\_protection\_enabled)

Description: Enable or disable AWS Shield Advanced protection for NAT EIPs. If set to 'true', a subscription to AWS Shield Advanced must exist in this account.

Type: `bool`

Default: `false`

### <a name="input_nat_gateway_enabled"></a> [nat\_gateway\_enabled](#input\_nat\_gateway\_enabled)

Description: Flag to enable/disable NAT gateways

Type: `bool`

Default: `true`

### <a name="input_nat_instance_ami_id"></a> [nat\_instance\_ami\_id](#input\_nat\_instance\_ami\_id)

Description: A list optionally containing the ID of the AMI to use for the NAT instance.  
If the list is empty (the default), the latest official AWS NAT instance AMI  
will be used. NOTE: The Official NAT instance AMI is being phased out and  
does not support NAT64. Use of a NAT gateway is recommended instead.

Type: `list(string)`

Default: `[]`

### <a name="input_nat_instance_enabled"></a> [nat\_instance\_enabled](#input\_nat\_instance\_enabled)

Description: Flag to enable/disable NAT instances

Type: `bool`

Default: `false`

### <a name="input_nat_instance_type"></a> [nat\_instance\_type](#input\_nat\_instance\_type)

Description: NAT Instance type

Type: `string`

Default: `"t3.micro"`

### <a name="input_public_subnets_enabled"></a> [public\_subnets\_enabled](#input\_public\_subnets\_enabled)

Description: If false, do not create public subnets.  
Since NAT gateways and instances must be created in public subnets, these will also not be created when `false`.

Type: `bool`

Default: `true`

### <a name="input_regex_replace_chars"></a> [regex\_replace\_chars](#input\_regex\_replace\_chars)

Description: Terraform regular expression (regex) string.  
Characters matching the regex will be removed from the ID elements.  
If not set, `"/[^a-zA-Z0-9-]/"` is used to remove all characters other than hyphens, letters and digits.

Type: `string`

Default: `null`

### <a name="input_stage"></a> [stage](#input\_stage)

Description: ID element. Usually used to indicate role, e.g. 'prod', 'staging', 'source', 'build', 'test', 'deploy', 'release'

Type: `string`

Default: `null`

### <a name="input_tags"></a> [tags](#input\_tags)

Description: Additional tags (e.g. `{'BusinessUnit': 'XYZ'}`).  
Neither the tag keys nor the tag values will be modified by this module.

Type: `map(string)`

Default: `{}`

### <a name="input_tenant"></a> [tenant](#input\_tenant)

Description: ID element \_(Rarely used, not included by default)\_. A customer identifier, indicating who this instance of a resource is for

Type: `string`

Default: `null`

### <a name="input_vpc_flow_logs_bucket_environment_name"></a> [vpc\_flow\_logs\_bucket\_environment\_name](#input\_vpc\_flow\_logs\_bucket\_environment\_name)

Description: The name of the environment where the VPC Flow Logs bucket is provisioned

Type: `string`

Default: `""`

### <a name="input_vpc_flow_logs_bucket_stage_name"></a> [vpc\_flow\_logs\_bucket\_stage\_name](#input\_vpc\_flow\_logs\_bucket\_stage\_name)

Description: The stage (account) name where the VPC Flow Logs bucket is provisioned

Type: `string`

Default: `""`

### <a name="input_vpc_flow_logs_bucket_tenant_name"></a> [vpc\_flow\_logs\_bucket\_tenant\_name](#input\_vpc\_flow\_logs\_bucket\_tenant\_name)

Description: The name of the tenant where the VPC Flow Logs bucket is provisioned.

If the `tenant` label is not used, leave this as `null`.

Type: `string`

Default: `null`

### <a name="input_vpc_flow_logs_enabled"></a> [vpc\_flow\_logs\_enabled](#input\_vpc\_flow\_logs\_enabled)

Description: Enable or disable the VPC Flow Logs

Type: `bool`

Default: `true`

### <a name="input_vpc_flow_logs_log_destination_type"></a> [vpc\_flow\_logs\_log\_destination\_type](#input\_vpc\_flow\_logs\_log\_destination\_type)

Description: The type of the logging destination. Valid values: `cloud-watch-logs`, `s3`

Type: `string`

Default: `"s3"`

### <a name="input_vpc_flow_logs_traffic_type"></a> [vpc\_flow\_logs\_traffic\_type](#input\_vpc\_flow\_logs\_traffic\_type)

Description: The type of traffic to capture. Valid values: `ACCEPT`, `REJECT`, `ALL`

Type: `string`

Default: `"ALL"`

## Outputs

The following outputs are exported:

### <a name="output_availability_zones"></a> [availability\_zones](#output\_availability\_zones)

Description: List of Availability Zones where subnets were created

### <a name="output_az_private_subnets_map"></a> [az\_private\_subnets\_map](#output\_az\_private\_subnets\_map)

Description: Map of AZ names to list of private subnet IDs in the AZs

### <a name="output_az_public_subnets_map"></a> [az\_public\_subnets\_map](#output\_az\_public\_subnets\_map)

Description: Map of AZ names to list of public subnet IDs in the AZs

### <a name="output_interface_vpc_endpoints"></a> [interface\_vpc\_endpoints](#output\_interface\_vpc\_endpoints)

Description: List of Interface VPC Endpoints in this VPC.

### <a name="output_max_subnet_count"></a> [max\_subnet\_count](#output\_max\_subnet\_count)

Description: Maximum allowed number of subnets before all subnet CIDRs need to be recomputed

### <a name="output_nat_eip_protections"></a> [nat\_eip\_protections](#output\_nat\_eip\_protections)

Description: List of AWS Shield Advanced Protections for NAT Elastic IPs.

### <a name="output_nat_gateway_ids"></a> [nat\_gateway\_ids](#output\_nat\_gateway\_ids)

Description: NAT Gateway IDs

### <a name="output_nat_gateway_public_ips"></a> [nat\_gateway\_public\_ips](#output\_nat\_gateway\_public\_ips)

Description: NAT Gateway public IPs

### <a name="output_nat_instance_ids"></a> [nat\_instance\_ids](#output\_nat\_instance\_ids)

Description: NAT Instance IDs

### <a name="output_private_route_table_ids"></a> [private\_route\_table\_ids](#output\_private\_route\_table\_ids)

Description: Private subnet route table IDs

### <a name="output_private_subnet_cidrs"></a> [private\_subnet\_cidrs](#output\_private\_subnet\_cidrs)

Description: Private subnet CIDRs

### <a name="output_private_subnet_ids"></a> [private\_subnet\_ids](#output\_private\_subnet\_ids)

Description: Private subnet IDs

### <a name="output_public_route_table_ids"></a> [public\_route\_table\_ids](#output\_public\_route\_table\_ids)

Description: Public subnet route table IDs

### <a name="output_public_subnet_cidrs"></a> [public\_subnet\_cidrs](#output\_public\_subnet\_cidrs)

Description: Public subnet CIDRs

### <a name="output_public_subnet_ids"></a> [public\_subnet\_ids](#output\_public\_subnet\_ids)

Description: Public subnet IDs

### <a name="output_route_tables"></a> [route\_tables](#output\_route\_tables)

Description: Route tables info map

### <a name="output_subnets"></a> [subnets](#output\_subnets)

Description: Subnets info map

### <a name="output_vpc"></a> [vpc](#output\_vpc)

Description: VPC info map

### <a name="output_vpc_cidr"></a> [vpc\_cidr](#output\_vpc\_cidr)

Description: VPC CIDR

### <a name="output_vpc_default_network_acl_id"></a> [vpc\_default\_network\_acl\_id](#output\_vpc\_default\_network\_acl\_id)

Description: The ID of the network ACL created by default on VPC creation

### <a name="output_vpc_default_security_group_id"></a> [vpc\_default\_security\_group\_id](#output\_vpc\_default\_security\_group\_id)

Description: The ID of the security group created by default on VPC creation

### <a name="output_vpc_id"></a> [vpc\_id](#output\_vpc\_id)

Description: VPC ID

