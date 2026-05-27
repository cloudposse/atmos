variable "cidr_block" {
  type        = string
  description = "CIDR block for the example VPC."
}

variable "tags" {
  type        = map(string)
  description = "Tags to attach to the example outputs."
  default     = {}
}
