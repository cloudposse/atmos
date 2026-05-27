terraform {
  required_version = ">= 1.6.0"
}

locals {
  tags = merge(
    {
      Component = "vpc"
      ManagedBy = "atmos"
    },
    var.tags,
  )
}

output "cidr_block" {
  value = var.cidr_block
}

output "tags" {
  value = local.tags
}
