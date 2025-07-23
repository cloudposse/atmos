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
  type = map(string)
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

source "amazon-ebs" "al2023" {
  ami_name      = var.ami_name
  source_ami    = var.source_ami
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username
  ami_org_arns = [var.org_arn]
  kms_key_id    = var.kms_key_arn
  encrypt_boot  = var.encrypt_boot

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

  # SSM Agent is pre-installed on AL2023 AMIs but should be enabled explicitly as done above.
  # MySQL client on AL2023 is installed via dnf install mysql (the mysql package includes the CLI tools).
  # `cloud-init clean` ensures the image will boot as a new instance on next launch.
  # `dnf clean all` removes cached metadata and packages to reduce AMI size.
  provisioner "shell" {
    inline = [
      # Enable and start the SSM agent (already installed by default on AL2023)
      "sudo systemctl enable --now amazon-ssm-agent",

      # Install packages, clean metadata and cloud-init
      "sudo -E bash -c 'dnf install -y jq && dnf clean all && cloud-init clean'"
    ]
  }

  # https://developer.hashicorp.com/packer/tutorials/docker-get-started/docker-get-started-post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors/manifest
  post-processor "manifest" {
    output     = var.manifest_file_name
    strip_path = var.manifest_strip_path
  }
}
