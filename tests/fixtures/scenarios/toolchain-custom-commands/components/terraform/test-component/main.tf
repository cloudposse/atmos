variable "environment" {
  type        = string
  description = "Environment name"
}

output "environment" {
  value = var.environment
}
