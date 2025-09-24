# Mock component for testing YAML functions in lists

variable "list_with_functions" {
  type        = list(string)
  description = "Test list containing YAML function outputs"
  default     = []
}

variable "mixed_list" {
  type        = list(string)
  description = "Test list with mixed YAML functions and static values"
  default     = []
}

variable "ecr_repository_arns" {
  type        = list(string)
  description = "Simulating the user's ECR repository ARNs case"
  default     = []
}

output "list_with_functions" {
  value = var.list_with_functions
}

output "mixed_list" {
  value = var.mixed_list
}

output "ecr_repository_arns" {
  value = var.ecr_repository_arns
}

# Simulating outputs for testing
output "repository_arn" {
  value = "arn:aws:ecr:us-east-1:123456789012:repository/${var.repo_name}"
}

variable "repo_name" {
  type    = string
  default = "test-repo"
}
