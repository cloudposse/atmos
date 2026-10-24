# Mock VPC component for demonstrating the terraform streaming UI (--ui).
# Uses null_resource + time_sleep so the cast recording needs no cloud
# credentials or emulator, but still gets a real multi-resource dependency
# tree and genuine multi-second create/destroy timing instead of completing
# instantly.

terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
    time = {
      source  = "hashicorp/time"
      version = "~> 0.9"
    }
  }
}

resource "null_resource" "vpc" {
  triggers = {
    cidr_block           = var.cidr_block
    environment          = var.environment
    enable_dns_hostnames = var.enable_dns_hostnames
    tags                 = jsonencode(var.tags)
  }
}

# Stands in for real cloud propagation delay between the VPC existing and its
# subnets being creatable/destroyable.
resource "time_sleep" "after_vpc" {
  depends_on       = [null_resource.vpc]
  create_duration  = var.vpc_propagation_delay
  destroy_duration = var.vpc_propagation_delay
}

resource "null_resource" "subnet" {
  for_each = var.subnets

  triggers = {
    cidr_block        = each.value.cidr_block
    availability_zone = each.value.availability_zone
    public            = each.value.public
  }

  depends_on = [time_sleep.after_vpc]
}

resource "time_sleep" "after_subnets" {
  depends_on       = [null_resource.subnet]
  create_duration  = var.subnet_propagation_delay
  destroy_duration = var.subnet_propagation_delay
}

resource "null_resource" "route_table_association" {
  for_each = var.subnets

  triggers = {
    subnet = each.key
  }

  depends_on = [time_sleep.after_subnets]
}
