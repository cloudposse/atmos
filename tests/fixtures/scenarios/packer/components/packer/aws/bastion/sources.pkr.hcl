source "amazon-ebs" "ubuntu" {
  ami_name      = "bastion"
  instance_type = "t4g.micro"
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
