# VPC Component Variables

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC"
  default     = "10.0.0.0/16"
}

variable "availability_zones" {
  type        = list(string)
  description = "List of availability zones to use"
  default     = ["us-east-1a", "us-east-1b"]
}

variable "nat_gateway_enabled" {
  type        = bool
  description = "Enable NAT Gateways for private subnets"
  default     = true
}

variable "dns_hostnames_enabled" {
  type        = bool
  description = "Enable DNS hostnames in the VPC"
  default     = true
}

variable "dns_support_enabled" {
  type        = bool
  description = "Enable DNS support in the VPC"
  default     = true
}

variable "max_subnet_count" {
  type        = number
  description = "Maximum number of subnets per type (public/private)"
  default     = 3
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to all resources"
  default     = {}
}
