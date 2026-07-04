# https://developer.hashicorp.com/packer/docs/templates/hcl_templates/blocks/source
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon/latest/components/builder/ebs
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon#authentication
# https://developer.hashicorp.com/packer/tutorials/docker-get-started/docker-get-started-post-processors
# https://developer.hashicorp.com/packer/tutorials/aws-get-started

packer {
  required_plugins {
    # https://developer.hashicorp.com/packer/integrations/hashicorp/amazon
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}

variable "region" {
  type        = string
  description = "AWS Region"
}

variable "stage" {
  type    = string
  default = null
}

variable "ami_org_arns" {
  type        = list(string)
  description = "List of Amazon Resource Names (ARN) of AWS Organizations that have access to launch the resulting AMI(s). By default no organizations have permission to launch the AMI"
  default     = []
}

variable "ami_ou_arns" {
  type        = list(string)
  description = "List of Amazon Resource Names (ARN) of AWS Organizations organizational units (OU) that have access to launch the resulting AMI(s). By default no organizational units have permission to launch the AMI."
  default     = []
}

variable "ami_users" {
  type        = list(string)
  description = "List of account IDs that have access to launch the resulting AMI(s). By default no additional users other than the user creating the AMI has permissions to launch it."
  default     = []
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

variable "source_ami" {
  type        = string
  description = "Source AMI"
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

variable "assume_role_session_name" {
  type        = string
  description = "Assume role session name"
}

variable "assume_role_duration_seconds" {
  type        = number
  description = "Assume role duration seconds"
}

variable "manifest_file_name" {
  type        = string
  description = "Manifest file name. Refer to https://developer.hashicorp.com/packer/docs/post-processors/manifest"
}

variable "manifest_strip_path" {
  type        = bool
  description = "Manifest strip path. Refer to https://developer.hashicorp.com/packer/docs/post-processors/manifest"
}

variable "associate_public_ip_address" {
  type        = bool
  description = "If this is `true`, the new instance will get a Public IP"
}

variable "provisioner_shell_commands" {
  type        = list(string)
  description = "List of commands to execute on the machine that Packer builds"
  default     = []
}

variable "force_deregister" {
  type        = bool
  description = "Force Packer to first deregister an existing AMI if one with the same name already exists"
  default     = false
}

variable "force_delete_snapshot" {
  type        = bool
  description = "Force Packer to delete snapshots associated with AMIs, which have been deregistered by `force_deregister`"
  default     = false
}

source "amazon-ebs" "al2023" {
  ami_name      = var.ami_name
  source_ami    = var.source_ami
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username
  ami_org_arns  = var.ami_org_arns
  ami_ou_arns   = var.ami_ou_arns
  ami_users     = var.ami_users
  kms_key_id    = var.kms_key_arn
  encrypt_boot  = var.encrypt_boot

  force_deregister      = var.force_deregister
  force_delete_snapshot = var.force_delete_snapshot

  associate_public_ip_address = var.associate_public_ip_address

  ami_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = var.volume_size
    volume_type           = var.volume_type
    delete_on_termination = true
  }

  assume_role {
    role_arn         = var.assume_role_arn
    session_name     = var.assume_role_session_name
    duration_seconds = var.assume_role_duration_seconds
  }

  aws_polling {
    delay_seconds = 5
    max_attempts  = 100
  }

  tags = var.ami_tags
}

build {
  sources = ["source.amazon-ebs.al2023"]

  provisioner "shell" {
    inline = var.provisioner_shell_commands
  }

  # https://developer.hashicorp.com/packer/tutorials/docker-get-started/docker-get-started-post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors/manifest
  post-processor "manifest" {
    output     = var.manifest_file_name
    strip_path = var.manifest_strip_path
  }
}
