variable "environment" {
  type        = string
  description = "Environment label."
}

variable "stage" {
  type        = string
  description = "Stack stage name from Atmos."
  default     = ""
}
