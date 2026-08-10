output "vpc_id" {
  description = "The fixture VPC ID."
  value       = aws_vpc.fixture.id
}

output "cidr_block" {
  description = "The fixture VPC CIDR block."
  value       = aws_vpc.fixture.cidr_block
}

output "name" {
  description = "The fixture VPC Name tag."
  value       = var.name
}
