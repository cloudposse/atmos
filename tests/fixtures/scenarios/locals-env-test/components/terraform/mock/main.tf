# Mock component for testing.

variable "test_env_value" {
  type        = string
  description = "Test environment variable value"
  default     = ""
}

variable "home_dir" {
  type        = string
  description = "Home directory from env"
  default     = ""
}

output "test_env_value" {
  value = var.test_env_value
}

output "home_dir" {
  value = var.home_dir
}
