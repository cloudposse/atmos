# Test fixture VPC. The app component intentionally does not create this VPC;
# its Terraform tests look it up after the app hook provisions this fixture.
resource "aws_vpc" "fixture" {
  cidr_block           = var.cidr_block
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name      = var.name
    ManagedBy = "atmos"
    Fixture   = "terraform-tests"
  }
}
