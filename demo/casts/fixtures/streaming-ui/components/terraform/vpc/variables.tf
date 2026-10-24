variable "cidr_block" {
  description = "The CIDR block for the VPC"
  type        = string
}

variable "environment" {
  description = "The environment name"
  type        = string
}

variable "enable_dns_hostnames" {
  description = "Whether to enable DNS hostnames in the VPC"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to the VPC"
  type        = map(string)
  default     = {}
}

variable "subnets" {
  description = "Subnets to create under the VPC, keyed by name"
  type = map(object({
    cidr_block        = string
    availability_zone = string
    public            = bool
  }))
  default = {
    public = {
      cidr_block        = "10.0.0.0/24"
      availability_zone = "us-east-2a"
      public            = true
    }
    private = {
      cidr_block        = "10.0.1.0/24"
      availability_zone = "us-east-2b"
      public            = false
    }
  }
}

variable "vpc_propagation_delay" {
  description = "Simulated propagation delay between the VPC and its subnets existing, so the streaming UI has real timing to display"
  type        = string
  default     = "600ms"
}

variable "subnet_propagation_delay" {
  description = "Simulated propagation delay between the subnets and their route table associations, so the streaming UI has real timing to display"
  type        = string
  default     = "600ms"
}
