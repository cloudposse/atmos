variable "org_arn" {
  type = string
}

variable "kms_key_arn" {
  type = string
}

packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}

source "amazon-ebs" "ubuntu" {
  ami_name      = "tester"
  instance_type = "t4g.micro"  # ARM64-compatible
  region        = "us-east-1"
  ssh_username  = "ubuntu"
  ami_org_arns  = [var.org_arn]
  kms_key_id    = var.kms_key_arn
  encrypt_boot  = true

  source_ami_filter {
    filters = {
      name                = "ubuntu/images/*/*24.04-arm64-server*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    owners      = ["099720109477"] # Canonical
    most_recent = true
  }

  ami_block_device_mappings {
    device_name = "/dev/xvda"
    volume_size = 8
    volume_type = "gp3"
    delete_on_termination = true
  }
}

build {
  sources = ["source.amazon-ebs.ubuntu"]

  provisioner "shell" {
    inline = [
      "sudo systemctl enable --now snap.amazon-ssm-agent.amazon-ssm-agent.service",
      "sudo -E bash -c 'export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y mysql-client && apt-get clean && cloud-init clean'",
    ]
  }

  post-processor "manifest" {
    output = "manifest.json"
    strip_path = true
  }
}
