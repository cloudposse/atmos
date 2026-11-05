variable "namespace" {
  description = "Namespace used in resource names."
  type        = string
}

variable "tags" {
  description = "Additional resource tags."
  type        = map(string)
  default     = {}
}
