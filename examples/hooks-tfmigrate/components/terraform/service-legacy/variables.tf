variable "environment" {
  type        = string
  description = "Environment label."
}

# Atmos passes stack-level vars into the generated tfvars file.
variable "stage" {
  type        = string
  description = "Stack stage name from Atmos."
  default     = ""
}
