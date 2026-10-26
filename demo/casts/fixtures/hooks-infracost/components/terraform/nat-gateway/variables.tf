variable "environment" {
  type        = string
  description = "Environment tag for all resources."
}

variable "stage" {
  type        = string
  description = "Stack stage name from Atmos."
  default     = ""
}
