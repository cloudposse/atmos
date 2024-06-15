variable "pet" {
  description = "The type of instance to include in the PetSet"
  type        = string
  default     = "dog"
}

variable "size" {
  description = "The number of pet instances to create"
  type        = number
  default     = 3
}
