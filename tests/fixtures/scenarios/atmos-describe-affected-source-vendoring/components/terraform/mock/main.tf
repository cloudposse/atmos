# Mock component for testing describe affected with source vendoring
variable "enabled" {
  type    = bool
  default = true
}

variable "name" {
  type    = string
  default = "mock"
}

output "name" {
  value = var.name
}
