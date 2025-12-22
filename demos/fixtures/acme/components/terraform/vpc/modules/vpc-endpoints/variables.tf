variable "vpc_id" {
  type        = string
  description = "VPC ID where the VPC Endpoints will be created (e.g. `vpc-aceb2723`)"
}

variable "gateway_vpc_endpoints" {
  type = map(object({
    name            = string
    policy          = string
    route_table_ids = list(string)
  }))
  description = <<-EOT
    A map of Gateway VPC Endpoints to provision into the VPC. This is a map of objects with the following attributes:
    - `name`: Short service name (either "s3" or "dynamodb")
    - `policy` = A policy (as JSON string) to attach to the endpoint that controls access to the service. May be `null` for full access.
    - `route_table_ids`: List of route tables to associate the gateway with. Routes to the gateway
      will be automatically added to these route tables.
    EOT
  default     = {}
}

variable "interface_vpc_endpoints" {
  type = map(object({
    name                = string
    policy              = string
    private_dns_enabled = bool
    security_group_ids  = list(string)
    subnet_ids          = list(string)
  }))
  description = <<-EOT
    A map of Interface VPC Endpoints to provision into the VPC.
    This is a map of objects with the following attributes:
    - `name`: Simple name of the service, like "ec2" or "redshift"
    - `policy`: A policy (as JSON string) to attach to the endpoint that controls access to the service. May be `null` for full access.
    - `private_dns_enabled`: Set `true` to associate a private hosted zone with the specified VPC
    - `security_group_ids`: The ID of one or more security groups to associate with the network interface.
      If empty list `[]` is provided, the VPC's default security group will be used automatically.
      If specific security group IDs are provided, they will replace the default security group association.
      To use both custom and default security groups, include both in the list.
    - `subnet_ids`: List of subnet in which to install the endpoints.
   EOT
  default     = {}
}

variable "post_creation_refresh_delay" {
  type        = string
  description = <<-EOT
    After endpoints are created via Terraform, some additional resources are provisioned by AWS
    that do not immediately appear in the resource, and therefore would not appear in the output
    of the resources attributes. Examples include private DNS entries and Network Interface IDs.
    This input (in `go` duration format) sets a time delay to allow for such activity, after which
    the endpoint data is fetched via data sources for output.
    EOT
  default     = "30s"
}
