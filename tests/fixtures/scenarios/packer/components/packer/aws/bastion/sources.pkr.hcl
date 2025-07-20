# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon/latest/components/builder/ebs

source "amazon-ebs" "this" {
  ami_name      = var.ami_name
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username
  ami_org_arns = [var.org_arn]
  kms_key_id    = var.kms_key_arn
  encrypt_boot  = var.encrypt_boot

  source_ami_filter {
    filters = {
      name                = "ubuntu/images/*/*24.04-arm64-server*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    owners = [var.ami_owner]
    most_recent = true
  }

  ami_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = var.volume_size
    volume_type           = var.volume_type
    delete_on_termination = true
  }
}
