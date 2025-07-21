variable "region" {
  type        = string
  description = "AWS Region"
}

variable "org_arn" {
  type        = string
  description = "Organization ARN"
}

variable "kms_key_arn" {
  type        = string
  description = "KMS Key ARN"
}

variable "instance_type" {
  type        = string
  description = "Instance type"
}

variable "volume_size" {
  type        = number
  description = "Volume size"
}

variable "volume_type" {
  type        = string
  description = "Volume type"
}

variable "ami_name" {
  type        = string
  description = "AMI name"
}

variable "ami_owner" {
  type        = string
  description = "AMI owner"
}

variable "ssh_username" {
  type        = string
  description = "Instance type"
}

variable "encrypt_boot" {
  type        = bool
  description = "Encrypt boot"
}

variable "skip_create_ami" {
  type        = bool
  description = "If true, Packer will not create the AMI. Useful for setting to true during a build test stage"
}

variable "ami_tags" {
  type        = map(string)
  description = "AMI tags"
}

# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon#authentication
variable "assume_role_arn" {
  type        = string
  description = "Amazon Resource Name (ARN) of the IAM Role to assume. Refer to https://developer.hashicorp.com/packer/integrations/hashicorp/amazon#authentication"
}

variable "manifest_file_name" {
  type        = string
  description = "Manifest file name. Refer to https://developer.hashicorp.com/packer/docs/post-processors/manifest"
}

variable "manifest_strip_path" {
  type        = bool
  description = "Manifest strip path. Refer to https://developer.hashicorp.com/packer/docs/post-processors/manifest"
}
