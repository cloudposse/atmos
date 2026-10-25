# Stand-in for a taggable resource. `terraform_data` is built into Terraform and
# OpenTofu, so no provider download is needed. The component applies `module.this.tags` to
# its resources, exactly as it would with the real module.
resource "terraform_data" "this" {
  input = module.this.tags
}
