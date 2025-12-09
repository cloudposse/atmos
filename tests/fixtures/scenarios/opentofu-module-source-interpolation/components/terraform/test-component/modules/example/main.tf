# Dummy module for testing
variable "test_input" {
  type    = string
  default = ""
}

output "test_output" {
  value = "Module received: ${var.test_input}"
}
