variable "region" {
  description = "AWS region."
  type        = string
  default     = "us-east-2"
}

variable "stage" {
  description = "Stage where the component is deployed (for example, dev or prod)."
  type        = string
}

variable "name" {
  description = "VPC name."
  type        = string
}

variable "ipv4_primary_cidr_block" {
  description = "IPv4 CIDR block for the VPC."
  type        = string
  default     = "10.9.0.0/18"
}

variable "availability_zones" {
  description = "List of Availability Zones in which to provision subnets."
  type        = list(string)
  default     = []
}

variable "map_public_ip_on_launch" {
  description = "Whether to assign a public IP address to instances launched in a subnet."
  type        = bool
  default     = false
}

variable "assign_generated_ipv6_cidr_block" {
  description = "Whether to request an Amazon-provided IPv6 CIDR block for the VPC."
  type        = bool
  default     = false
}
