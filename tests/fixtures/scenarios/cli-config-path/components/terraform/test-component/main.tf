variable "enabled" {
  type    = bool
  default = false
}

output "enabled" {
  value = var.enabled
}
