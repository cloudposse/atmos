# Base component for plan-diff metadata.component test
variable "name" {
  type        = string
  description = "Name of the component instance"
}

variable "custom_var" {
  type        = string
  description = "Custom variable for derived component"
  default     = ""
}

output "name" {
  value = var.name
}

output "custom_var" {
  value = var.custom_var
}
