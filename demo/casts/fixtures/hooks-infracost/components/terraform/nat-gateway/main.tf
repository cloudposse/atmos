# A NAT gateway is one of the most expensive "small" AWS resources, which
# makes it a good demo target for infracost — it shows a non-zero monthly
# cost without provisioning anything in a real cloud account.

resource "aws_eip" "nat" {
  domain = "vpc"
  tags = {
    Name        = "nat-eip"
    Environment = var.environment
  }
}

resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  subnet_id     = "subnet-deadbeef" # placeholder — not provisioned
  tags = {
    Name        = "main-nat"
    Environment = var.environment
  }
}
