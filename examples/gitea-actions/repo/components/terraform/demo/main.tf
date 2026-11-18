# A provider-free component: terraform_data is built into Terraform and OpenTofu,
# so plans and applies need no cloud credentials and no provider downloads.
variable "stage" {
  type        = string
  description = "The stack's stage (dev or prod), passed by Atmos from the stack vars."
}

variable "message" {
  type        = string
  description = "A message stored in state."
}

resource "terraform_data" "message" {
  input = var.message
}

output "message" {
  value       = terraform_data.message.output
  description = "The stored message."
}
