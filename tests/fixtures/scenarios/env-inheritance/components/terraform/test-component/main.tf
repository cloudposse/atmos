# Dummy terraform component for testing
variable "enabled" {
  type    = bool
  default = true
}

variable "name" {
  type = string
}

variable "stage" {
  type = string
}

output "test_output" {
  value = "test"
}
