################################
# terraform-null-label example #
################################
module "label" {
  source = "../../"

  context = module.this.context
}

#######################
# Launch template     #
#######################
resource "aws_launch_template" "default" {
  # terraform-null-label example used here: Set template name prefix
  name_prefix                          = "${module.label.id}-"
  image_id                             = data.aws_ami.amazon_linux.id
  instance_type                        = "t2.micro"
  instance_initiated_shutdown_behavior = "terminate"

  vpc_security_group_ids = [data.aws_security_group.default.id]

  monitoring {
    enabled = false
  }

  # terraform-null-label example used here: Set tags on everything that can be tagged
  tag_specifications {
    for_each = ["instance", "volume", "elastic-gpu", "spot-instance-request"]

    resource_type = each.value
    tags          = module.label.tags
  }

  # Bridgecrew BC_AWS_GENERAL_26
  tags = module.label.tags

  # Bridgecrew compliance: Ensure Instance Metadata Service Version 1 is not enabled (BC_AWS_GENERAL_31)
  metadata_options {
    http_tokens = "required"
  }
}

######################
# Autoscaling group  #
######################
resource "aws_autoscaling_group" "default" {
  # terraform-null-label example used here: Set ASG name prefix
  name_prefix         = "${module.label.id}-"
  vpc_zone_identifier = data.aws_subnet_ids.all.ids
  max_size            = "1"
  min_size            = "1"
  desired_capacity    = "1"

  launch_template {
    id      = aws_launch_template.default.id
    version = "$Latest"
  }

  # terraform-null-label example used here: Set tags on ASG and EC2 Servers
  tags = module.label.tags_as_list_of_maps
}

################################
# Provider                     #
################################
provider "aws" {
  region = "eu-west-1"

  # Make it faster by skipping unneeded checks here
  skip_get_ec2_platforms      = true
  skip_metadata_api_check     = true
  skip_region_validation      = true
  skip_credentials_validation = true
  skip_requesting_account_id  = true
}

##############################################################
# Data sources to get VPC, subnets and security group details
##############################################################
data "aws_vpc" "default" {
  default = true
}

data "aws_subnet_ids" "all" {
  vpc_id = data.aws_vpc.default.id
}

data "aws_security_group" "default" {
  vpc_id = data.aws_vpc.default.id
  name   = "default"
}

data "aws_ami" "amazon_linux" {
  most_recent = true

  owners = ["amazon"]

  filter {
    name = "name"

    values = [
      "amzn-ami-hvm-*-x86_64-gp2",
    ]
  }

  filter {
    name = "owner-alias"

    values = [
      "amazon",
    ]
  }
}

