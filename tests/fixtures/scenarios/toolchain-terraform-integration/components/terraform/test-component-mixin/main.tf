variable "environment" {
  type        = string
  description = "Environment name"
}

variable "test_var" {
  type        = string
  description = "Test variable"
}

output "environment" {
  value       = var.environment
  description = "The environment name"
}

output "test_var" {
  value       = var.test_var
  description = "The test variable value"
}

output "terraform_version" {
  value       = "Terraform ${terraform.version}"
  description = "Terraform version being used"
}
