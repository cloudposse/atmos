# These resources are in outputs.tf because they are solely for producing output

#  After endpoints are created via Terraform, some additional resources are provisioned by AWS
#  that do not immediately appear in the resource, and therefore would not appear in the output
#  of the resources attributes. Examples include private DNS entries and Network Interface IDs.
#  We cannot refresh the resources and prevent Terraform from showing drift after creation,
#  but we can wait and populate the output from data sources to capture this additional information.

resource "time_sleep" "creation" {
  count = local.enabled ? 1 : 0

  depends_on = [
    aws_vpc_endpoint.gateway_endpoint,
    aws_vpc_endpoint_route_table_association.gateway,
    aws_vpc_endpoint.interface_endpoint,
    aws_vpc_endpoint_subnet_association.interface,
  ]

  create_duration = var.post_creation_refresh_delay
}

data "aws_vpc_endpoint" "gateway" {
  for_each = local.enabled ? var.gateway_vpc_endpoints : {}

  id = aws_vpc_endpoint.gateway_endpoint[each.key].id

  depends_on = [time_sleep.creation]
}

data "aws_vpc_endpoint" "interface" {
  for_each = local.enabled ? var.interface_vpc_endpoints : {}

  id = aws_vpc_endpoint.interface_endpoint[each.key].id

  depends_on = [time_sleep.creation]
}

output "gateway_vpc_endpoints_map" {
  value       = data.aws_vpc_endpoint.gateway
  description = "Map of Gateway VPC Endpoints deployed to this VPC, using keys supplied in `var.gateway_vpc_endpoints`."
}

output "interface_vpc_endpoints_map" {
  value       = data.aws_vpc_endpoint.interface
  description = "Map of Interface VPC Endpoints deployed to this VPC, using keys supplied in `var.interface_vpc_endpoints`."
}
