variable "name" {
  type        = string
  description = "Base name for the resources."
}

variable "environment" {
  type        = string
  description = "Environment suffix appended to resource names."
  default     = "test"
}

variable "stage" {
  type        = string
  description = "Atmos stack stage. Accepted so shared stack vars do not produce Terraform warnings."
  default     = null
}

variable "enable_versioning" {
  type        = bool
  description = "Whether to enable S3 bucket versioning."
  default     = true
}

variable "fixture_vpc_name" {
  type        = string
  description = "Name tag of the fixture VPC that must already exist."
  default     = null
}
