# Display the module's tag map without creating cloud resources.
resource "terraform_data" "this" {
  input = module.this.tags
}
