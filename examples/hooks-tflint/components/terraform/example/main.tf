# A provider-free resource so the component plans offline with NO cloud
# credentials — the point of this example is the linter, not the resource.
# tflint reads static HCL, so it lints regardless of what's being provisioned.
resource "terraform_data" "example" {
  input = {
    environment = var.environment
    stage       = var.stage
  }
}
