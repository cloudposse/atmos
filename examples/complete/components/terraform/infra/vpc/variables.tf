variable "region" {
  type        = string
  description = "AWS Region"
}

variable "availability_zones" {
  type        = list(string)
  default     = []
  description = "List of availability zones in which to provision VPC subnets"
}

variable "region_availability_zones" {
  type        = list(string)
  default     = []
  description = "List of availability zones in region, to be used as default when `availability_zones` is not supplied"
}

variable "cidr_block" {
  type        = string
  description = "VPC CIDR block"
}

variable "nat_gateway_enabled" {
  type        = bool
  description = "Flag to enable/disable NAT gateways"
}

variable "nat_instance_enabled" {
  type        = bool
  description = "Flag to enable/disable NAT instances"
}

variable "nat_instance_type" {
  type        = string
  description = "NAT Instance type"
  default     = "t3.micro"
}

variable "map_public_ip_on_launch" {
  type        = bool
  default     = true
  description = "Instances launched into a public subnet should be assigned a public IP address"
}

variable "subnet_type_tag_key" {
  type        = string
  description = "Key for subnet type tag to provide information about the type of subnets, e.g. `cpco/subnet/type=private` or `cpcp/subnet/type=public`"
}

variable "subnet_type_tag_value_format" {
  type        = string
  default     = "%s"
  description = "This is using the format interpolation symbols to allow the value of the subnet_type_tag_key to be modified"
}

variable "max_subnet_count" {
  type        = number
  default     = 0
  description = "Sets the maximum amount of subnets to deploy. 0 will deploy a subnet for every provided availability zone (in `region_availability_zones` variable) within the region"
}
