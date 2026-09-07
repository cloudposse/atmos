variable "environment" {
  type        = string
  description = "Fed into the example resource."
}

# Atmos passes stack-level vars (stage) into the tfvars file; declaring
# it here silences the "Value for undeclared variable" warning from
# OpenTofu without requiring stack-side filtering.
variable "stage" {
  type        = string
  description = "Stack stage name from Atmos."
  default     = ""
}

# Intentionally unused: tflint's builtin terraform_unused_declarations
# rule flags this, giving the demo a deterministic lint finding.
variable "unused" {
  type        = string
  description = "Declared but never referenced — demonstrates a tflint finding."
  default     = ""
}
