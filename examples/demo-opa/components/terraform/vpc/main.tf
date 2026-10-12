# A credential-free mock of a VPC. It derives one subnet CIDR per Availability
# Zone from the primary CIDR block using built-in functions only, so there are no
# providers or cloud resources to configure. The point of this example is the OPA
# validation in `stacks/schemas/opa/vpc`, not the Terraform itself.

locals {
  # Carve one /24-style subnet out of the primary CIDR for each Availability Zone.
  subnet_cidrs = [
    for index, _ in var.availability_zones :
    cidrsubnet(var.ipv4_primary_cidr_block, 6, index)
  ]
}
