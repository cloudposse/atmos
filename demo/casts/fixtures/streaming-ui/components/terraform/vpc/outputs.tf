output "vpc_cidr" {
  description = "The CIDR block of the VPC"
  value       = var.cidr_block
}

output "environment" {
  description = "The environment name"
  value       = var.environment
}

output "subnet_ids" {
  description = "The (mock) subnet identifiers created under the VPC"
  value       = { for name, subnet in null_resource.subnet : name => subnet.id }
}
