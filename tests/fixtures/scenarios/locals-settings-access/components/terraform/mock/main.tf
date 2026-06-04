# Mock Terraform component for testing.
variable "domain" {
  type        = string
  description = "Domain name"
  default     = ""
}

variable "substage" {
  type        = string
  description = "Substage"
  default     = ""
}

output "domain" {
  value = var.domain
}

output "substage" {
  value = var.substage
}
