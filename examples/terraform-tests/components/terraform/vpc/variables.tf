variable "name" {
  type        = string
  description = "Name tag for the fixture VPC."
}

variable "cidr_block" {
  type        = string
  description = "CIDR block for the fixture VPC."
  default     = "10.99.0.0/16"

  validation {
    condition     = can(cidrhost(var.cidr_block, 0))
    error_message = "cidr_block must be a valid IPv4 or IPv6 CIDR block."
  }
}

variable "stage" {
  type        = string
  description = "Atmos stack stage. Accepted so shared stack vars do not produce Terraform warnings."
  default     = null
}
