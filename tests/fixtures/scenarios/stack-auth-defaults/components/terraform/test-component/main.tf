# Minimal test component
variable "enabled" {
  type    = bool
  default = true
}

output "enabled" {
  value = var.enabled
}
