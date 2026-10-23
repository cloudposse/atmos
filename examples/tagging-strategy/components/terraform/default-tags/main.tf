# Stand-in for a taggable AWS resource. `terraform_data` is built into Terraform and
# OpenTofu, so no provider download is needed.
resource "terraform_data" "this" {
  input = var.name
}
