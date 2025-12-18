# Test component for issue #1858 fixture
variable "enabled" {
  type    = bool
  default = true
}

output "enabled" {
  value = var.enabled
}
