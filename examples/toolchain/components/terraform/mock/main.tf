# Mock component for toolchain demo.
#
# This is a minimal component that demonstrates component-level tool dependencies.
# The component itself does nothing except output a message, but it shows how
# Atmos can install and manage tool versions per-component.

variable "message" {
  type        = string
  description = "Message to output"
  default     = "Hello from mock component"
}

output "message" {
  description = "The configured message"
  value       = var.message
}
