# A local resource keeps affected detection independent of cloud APIs.
resource "terraform_data" "app" {
  input = {
    stage    = var.stage
    location = var.location
  }
}
