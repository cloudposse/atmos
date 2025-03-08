variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "test" {
  description = "Test variable"
  type        = map(string)
  default     = {}
}
