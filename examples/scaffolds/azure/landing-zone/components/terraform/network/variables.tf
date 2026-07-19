variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "location" {
  description = "Azure location for resources."
  type        = string
}

variable "address_space" {
  description = "Address space for the virtual network."
  type        = list(string)
}

variable "subnet_prefixes" {
  description = "CIDR prefixes for the default subnet."
  type        = list(string)
}
