# Variables for multi-file Packer component test.
# This file tests that Atmos correctly supports directory-based templates
# where Packer loads all *.pkr.hcl files from the component directory.

variable "region" {
  type        = string
  description = "AWS Region"
}

variable "stage" {
  type    = string
  default = null
}

variable "ami_name" {
  type        = string
  description = "AMI name"
}

variable "source_ami" {
  type        = string
  description = "Source AMI"
}

variable "instance_type" {
  type        = string
  description = "Instance type"
}

variable "ssh_username" {
  type        = string
  description = "SSH username"
}

variable "skip_create_ami" {
  type        = bool
  description = "If true, Packer will not create the AMI. Useful for setting to true during a build test stage"
  default     = true
}

variable "manifest_file_name" {
  type        = string
  description = "Manifest file name"
  default     = "manifest.json"
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to the AMI"
  default     = {}
}
