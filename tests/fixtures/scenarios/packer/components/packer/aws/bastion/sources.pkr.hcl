# https://developer.hashicorp.com/packer/docs/templates/hcl_templates/blocks/source
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon/latest/components/builder/ebs
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon
# https://developer.hashicorp.com/packer/integrations/hashicorp/amazon#authentication
# https://developer.hashicorp.com/packer/tutorials/docker-get-started/docker-get-started-post-processors
# https://developer.hashicorp.com/packer/tutorials/aws-get-started

source "amazon-ebs" "this" {
  ami_name      = var.ami_name
  source_ami    = var.source_ami
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username
  ami_org_arns = [var.org_arn]
  kms_key_id    = var.kms_key_arn
  encrypt_boot  = var.encrypt_boot

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

  tags = var.ami_tags
}
