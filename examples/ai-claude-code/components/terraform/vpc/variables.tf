variable "vpc_cidr" {
  type        = string
  description = "VPC CIDR block."
  default     = "10.0.0.0/16"
}

variable "availability_zones" {
  type        = list(string)
  description = "Availability zones for subnets."
  default     = ["us-east-2a", "us-east-2b"]
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to resources."
  default     = {}
}
