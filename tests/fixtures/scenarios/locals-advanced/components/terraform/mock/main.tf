# Mock Terraform component for testing locals
# This component accepts any vars and outputs them for verification

variable "name" {
  type        = string
  default     = ""
  description = "Resource name"
}

variable "cidr" {
  type        = string
  default     = ""
  description = "CIDR block"
}

variable "region" {
  type        = string
  default     = ""
  description = "AWS region"
}

variable "db_identifier" {
  type        = string
  default     = ""
  description = "Database identifier"
}

variable "env_suffix" {
  type        = string
  default     = ""
  description = "Environment suffix"
}

variable "backend_bucket" {
  type        = string
  default     = ""
  description = "Backend bucket name"
}

variable "home_dir" {
  type        = string
  default     = ""
  description = "Home directory from env"
}

variable "username" {
  type        = string
  default     = ""
  description = "Username from env"
}

output "name" {
  value = var.name
}

output "region" {
  value = var.region
}
