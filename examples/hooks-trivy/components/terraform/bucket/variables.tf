variable "environment" {
  type        = string
  description = "Environment tag."
}

# Atmos passes stack-level vars (stage) into the tfvars file; declaring
# it here silences the "Value for undeclared variable" warning from
# OpenTofu without requiring stack-side filtering.
variable "stage" {
  type        = string
  description = "Stack stage name from Atmos."
  default     = ""
}
