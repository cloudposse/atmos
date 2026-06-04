# Main Packer template for multi-file component test.
# This file uses variables defined in variables.pkr.hcl to verify
# that Atmos correctly supports directory-based templates.

packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}

source "amazon-ebs" "test" {
  ami_name      = var.ami_name
  source_ami    = var.source_ami
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username

  skip_create_ami = var.skip_create_ami

  tags = var.tags
}

build {
  sources = ["source.amazon-ebs.test"]

  post-processor "manifest" {
    output     = var.manifest_file_name
    strip_path = true
  }
}
